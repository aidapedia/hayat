package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/store"
)

// A Session is one signed-in browser on Hayat's sign-in domain. Its cookie is
// what gives single sign-on: every service that sends the browser to Hayat
// finds the session and gets a code without a prompt.
type Session struct {
	ID         uuid.UUID
	Region     string
	UserID     uuid.UUID
	AMR        []string
	IP         string
	UserAgent  string
	AuthTime   time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

func (s Session) Active() bool { return s.RevokedAt == nil && time.Now().Before(s.ExpiresAt) }

var ErrNoSession = errors.New("no session")

const sessionColumns = `id, region, user_id, amr, ip, user_agent, auth_time, last_seen_at, expires_at, revoked_at, created_at`

func scanSession(row pgx.Row) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.Region, &s.UserID, &s.AMR, &s.IP, &s.UserAgent, &s.AuthTime, &s.LastSeenAt,
		&s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	return s, store.NotFound(err)
}

// CreateSession signs a user in and returns the cookie value. The value is
// "<region>.<token>", so any region can find the session in its home region.
func (s *Service) CreateSession(ctx context.Context, u User, amr []string, ip, userAgent string) (string, Session, error) {
	token := crypto.NewToken()
	now := time.Now()
	if len(userAgent) > 400 {
		userAgent = userAgent[:400]
	}
	var sess Session
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		sess, err = scanSession(tx.QueryRow(ctx, `INSERT INTO sessions
			(region, id, user_id, token_hash, amr, ip, user_agent, auth_time, last_seen_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9) RETURNING `+sessionColumns,
			u.Region, store.NewID(), u.ID, crypto.HashToken(token), nonNil(amr), ip, userAgent, now, now.Add(s.sessionTTL)))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, audit.UserActor(u.ID), "session.created", "session:"+sess.ID.String(), map[string]any{"ip": ip, "amr": amr})
	})
	if err != nil {
		return "", Session{}, err
	}
	return u.Region + "." + token, sess, nil
}

// SessionByCookie returns the live session and its active user, or ErrNoSession.
func (s *Service) SessionByCookie(ctx context.Context, cookie string) (Session, User, error) {
	region, token, ok := strings.Cut(cookie, ".")
	if !ok || region == "" || token == "" {
		return Session{}, User{}, ErrNoSession
	}
	sess, err := scanSession(s.store.Pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE region = $1 AND token_hash = $2`, region, crypto.HashToken(token)))
	if err != nil || !sess.Active() {
		return Session{}, User{}, ErrNoSession
	}
	u, err := s.GetUser(ctx, sess.UserID)
	if err != nil || u.Status != StatusActive {
		return Session{}, User{}, ErrNoSession
	}
	// Recording every request would turn reads into writes; once every few
	// minutes is enough for "last active".
	if time.Since(sess.LastSeenAt) > 5*time.Minute {
		_, _ = s.store.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE region = $1 AND id = $2`, sess.Region, sess.ID)
	}
	return sess, u, nil
}

// SessionByID is how token holders are checked: a token is only good while the
// session it was issued under is alive and its user is active.
func (s *Service) SessionByID(ctx context.Context, region string, id uuid.UUID) (Session, error) {
	return scanSession(s.store.Pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE region = $1 AND id = $2`, region, id))
}

func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now() ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Service) CountActiveSessions(ctx context.Context) (int, error) {
	var n int
	err := s.store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > now()`).Scan(&n)
	return n, err
}

// RevokeSession ends one session of a user.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now()
			WHERE region = $1 AND id = $2 AND user_id = $3 AND revoked_at IS NULL`, region, sessionID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return store.ErrNotFound
		}
		return audit.Write(ctx, tx, region, actor, "session.revoked", "session:"+sessionID.String(), nil)
	})
}

// RevokeAllSessions signs a user out everywhere.
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := revokeAllSessions(ctx, tx, region, userID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "session.revoked_all", "user:"+userID.String(), nil)
	})
}

func revokeAllSessions(ctx context.Context, tx pgx.Tx, region string, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE region = $1 AND user_id = $2 AND revoked_at IS NULL`, region, userID)
	return err
}

// EndSessionByCookie signs out the browser holding this cookie.
func (s *Service) EndSessionByCookie(ctx context.Context, cookie string) {
	region, token, ok := strings.Cut(cookie, ".")
	if !ok {
		return
	}
	_, _ = s.store.Pool.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE region = $1 AND token_hash = $2 AND revoked_at IS NULL`, region, crypto.HashToken(token))
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
