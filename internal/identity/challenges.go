package identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/store"
)

// A Challenge is a sign-in half done, held between two pages by a cookie:
//   - mfa: the first factor was right; a second one is next.
//   - enroll: the first factor was right, but a tenant requires a second
//     factor this person hasn't set up yet.
//   - passkey: a WebAuthn ceremony is under way (Data holds its session).
type Challenge struct {
	ID            uuid.UUID
	Region        string
	Kind          string
	UserID        *uuid.UUID
	AMR           []string
	AuthRequestID string
	Data          json.RawMessage
	Attempts      int
	ExpiresAt     time.Time
}

const (
	ChallengeMFA     = "mfa"
	ChallengeEnroll  = "enroll"
	ChallengePasskey = "passkey"

	// MaxChallengeAttempts wrong codes end a challenge; the person signs in again.
	MaxChallengeAttempts = 5
)

var ErrNoChallenge = errors.New("no challenge")

const challengeColumns = `id, region, kind, user_id, amr, auth_request_id, data, attempts, expires_at`

func scanChallenge(row pgx.Row) (Challenge, error) {
	var c Challenge
	var data []byte
	err := row.Scan(&c.ID, &c.Region, &c.Kind, &c.UserID, &c.AMR, &c.AuthRequestID, &data, &c.Attempts, &c.ExpiresAt)
	c.Data = data
	return c, store.NotFound(err)
}

// NewChallenge starts a challenge and returns the value its cookie carries,
// "<region>.<token>" like a session's.
func (s *Service) NewChallenge(ctx context.Context, kind string, userID *uuid.UUID, amr []string, authRequestID string, data any, ttl time.Duration) (string, Challenge, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", Challenge{}, err
	}
	if data == nil {
		raw = []byte("{}")
	}
	token := crypto.NewToken()
	c, err := scanChallenge(s.store.Pool.QueryRow(ctx, `INSERT INTO sign_in_challenges
		(region, id, token_hash, kind, user_id, amr, auth_request_id, data, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9) RETURNING `+challengeColumns,
		s.store.Region, store.NewID(), crypto.HashToken(token), kind, userID, nonNil(amr), authRequestID, string(raw), time.Now().Add(ttl)))
	if err != nil {
		return "", Challenge{}, err
	}
	return s.store.Region + "." + token, c, nil
}

// ChallengeByCookie returns the live challenge of this kind, or ErrNoChallenge.
func (s *Service) ChallengeByCookie(ctx context.Context, cookie, kind string) (Challenge, error) {
	region, token, ok := strings.Cut(cookie, ".")
	if !ok || region == "" || token == "" {
		return Challenge{}, ErrNoChallenge
	}
	c, err := scanChallenge(s.store.Pool.QueryRow(ctx, `SELECT `+challengeColumns+` FROM sign_in_challenges
		WHERE region = $1 AND token_hash = $2 AND kind = $3 AND expires_at > now() AND attempts < $4`,
		region, crypto.HashToken(token), kind, MaxChallengeAttempts))
	if err != nil {
		return Challenge{}, ErrNoChallenge
	}
	return c, nil
}

// FailChallenge counts a wrong answer and reports how many tries are left.
func (s *Service) FailChallenge(ctx context.Context, c Challenge) (int, error) {
	var n int
	err := s.store.Pool.QueryRow(ctx, `UPDATE sign_in_challenges SET attempts = attempts + 1
		WHERE region = $1 AND id = $2 RETURNING attempts`, c.Region, c.ID).Scan(&n)
	return MaxChallengeAttempts - n, err
}

// SetChallengeData replaces what a challenge carries (a fresh WebAuthn session).
func (s *Service) SetChallengeData(ctx context.Context, c Challenge, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.store.Pool.Exec(ctx, `UPDATE sign_in_challenges SET data = $3::jsonb WHERE region = $1 AND id = $2`, c.Region, c.ID, string(raw))
	return err
}

// EndChallenge uses a challenge up. It reports false when someone else used
// it first, so a challenge finishes exactly one sign-in.
func (s *Service) EndChallenge(ctx context.Context, c Challenge) (bool, error) {
	tag, err := s.store.Pool.Exec(ctx, `DELETE FROM sign_in_challenges WHERE region = $1 AND id = $2`, c.Region, c.ID)
	return tag.RowsAffected() == 1, err
}
