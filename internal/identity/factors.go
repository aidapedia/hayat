package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is HMAC-SHA1, which every authenticator app speaks
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	hmail "github.com/aidapedia/hayat/internal/mail"
	"github.com/aidapedia/hayat/internal/store"
)

// A Factor is an authenticator app or a passkey someone set up. Either one
// is a second factor; a passkey can also sign in on its own.
type Factor struct {
	ID         uuid.UUID
	Kind       string // totp | passkey
	Name       string
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

const (
	FactorTOTP    = "totp"
	FactorPasskey = "passkey"

	totpPeriod = 30
	totpDigits = 6
	// A code from the step before or after also counts, for clocks a little off.
	totpSkew = 1

	recoveryCodeCount = 10
)

var (
	ErrBadCode        = errs.New("that code isn't right: check the app and try again")
	ErrNoBox          = errs.New("authenticator apps aren't available on this server")
	ErrLastFactor     = errs.New("a tenant you belong to requires two-step verification: add another method before removing this one")
	ErrLastWayIn      = errs.New("this is your only way to sign in: set a password or link another account first")
	ErrTooManyFactors = errs.New("you have set up as many sign-in methods as Hayat allows")
)

const factorColumns = `id, kind, name, last_used_at, created_at`

func scanFactor(row pgx.Row) (Factor, error) {
	var f Factor
	err := row.Scan(&f.ID, &f.Kind, &f.Name, &f.LastUsedAt, &f.CreatedAt)
	return f, store.NotFound(err)
}

// Factors lists the authenticator apps and passkeys a person has set up.
func (s *Service) Factors(ctx context.Context, userID uuid.UUID) ([]Factor, error) {
	region, err := s.regionOf(ctx, s.store.Pool, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Pool.Query(ctx, `SELECT `+factorColumns+` FROM user_factors
		WHERE region = $1 AND user_id = $2 AND confirmed ORDER BY created_at`, region, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Factor
	for rows.Next() {
		f, err := scanFactor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// HasSecondFactor reports whether signing in with a password (or another
// account) must be followed by a code or a passkey.
func (s *Service) HasSecondFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_factors WHERE user_id = $1 AND confirmed)`, userID).Scan(&ok)
	return ok, err
}

// HasTOTP reports whether the person can type a code from an authenticator app.
func (s *Service) HasTOTP(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_factors WHERE user_id = $1 AND kind = 'totp' AND confirmed)`, userID).Scan(&ok)
	return ok, err
}

// RemoveFactor deletes one of a person's factors. keepOne refuses to remove
// the last second factor (a tenant requires one); keepWayIn refuses to remove
// the last passkey of an account with no other way in. Removing the last
// factor also voids the recovery codes: there is nothing left to recover.
func (s *Service) RemoveFactor(ctx context.Context, userID, factorID uuid.UUID, keepOne, keepWayIn bool, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM user_factors WHERE region = $1 AND id = $2 AND user_id = $3 AND confirmed FOR UPDATE`,
			u.Region, factorID, userID).Scan(&kind); err != nil {
			return store.NotFound(err)
		}
		var total, passkeys int
		if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE kind = 'passkey') FROM user_factors
			WHERE region = $1 AND user_id = $2 AND confirmed`, u.Region, userID).Scan(&total, &passkeys); err != nil {
			return err
		}
		if keepOne && total == 1 {
			return ErrLastFactor
		}
		if keepWayIn && kind == FactorPasskey && passkeys == 1 {
			return ErrLastWayIn
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_factors WHERE region = $1 AND id = $2`, u.Region, factorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM passkey_refs WHERE factor_id = $1`, factorID); err != nil {
			return err
		}
		if total == 1 {
			if _, err := tx.Exec(ctx, `DELETE FROM recovery_codes WHERE region = $1 AND user_id = $2`, u.Region, userID); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, u.Region, actor, "user.factor_removed", "user:"+userID.String(), map[string]any{"kind": kind}); err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "A sign-in method was removed from your Hayat account",
			fmt.Sprintf("Hi %s,\n\nA %s was just removed from your account.\n\nIf this wasn't you, reset your password now: %s/forgot", u.DisplayName(), factorLabel(kind), s.issuer))
	})
}

// ResetFactors removes every factor and recovery code of someone who lost
// them all, and signs them out everywhere: an instance admin's last resort.
func (s *Service) ResetFactors(ctx context.Context, userID uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM passkey_refs WHERE factor_id IN (SELECT id FROM user_factors WHERE region = $1 AND user_id = $2)`,
			`DELETE FROM user_factors WHERE region = $1 AND user_id = $2`,
			`DELETE FROM recovery_codes WHERE region = $1 AND user_id = $2`,
		} {
			if _, err := tx.Exec(ctx, q, u.Region, userID); err != nil {
				return err
			}
		}
		if err := revokeAllSessions(ctx, tx, u.Region, userID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, u.Region, actor, "user.factors_reset", "user:"+userID.String(), nil); err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "Two-step verification was reset on your Hayat account",
			fmt.Sprintf("Hi %s,\n\nAn administrator removed your authenticator apps, passkeys and recovery codes, and signed you out everywhere. Sign in again and set up two-step verification from Account → Two-step verification.\n\nIf you didn't ask for this, contact your administrator.", u.DisplayName()))
	})
}

func factorLabel(kind string) string {
	if kind == FactorPasskey {
		return "passkey"
	}
	return "authenticator app"
}

// ---------- Authenticator apps (TOTP, RFC 6238) ----------

// TOTPSetup is what the person scans: the secret, as text and as an
// otpauth:// address for a QR code.
type TOTPSetup struct {
	FactorID uuid.UUID
	Secret   string
	URI      string
}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// BeginTOTP makes a new secret for an authenticator app. It counts once the
// person types a code from it (ConfirmTOTP); a setup left unfinished is
// replaced by the next one.
func (s *Service) BeginTOTP(ctx context.Context, u User) (TOTPSetup, error) {
	if s.sec.Box == nil {
		return TOTPSetup{}, ErrNoBox
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return TOTPSetup{}, err
	}
	id := store.NewID()
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.checkFactorRoom(ctx, tx, u); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_factors WHERE region = $1 AND user_id = $2 AND kind = 'totp' AND NOT confirmed`, u.Region, u.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_factors (region, id, user_id, kind, name, secret_sealed)
			VALUES ($1, $2, $3, 'totp', 'Authenticator app', $4)`, u.Region, id, u.ID, s.sec.Box.Seal(secret))
		return err
	})
	if err != nil {
		return TOTPSetup{}, err
	}
	enc := b32.EncodeToString(secret)
	label := s.otpIssuer()
	v := url.Values{}
	v.Set("secret", enc)
	v.Set("issuer", label)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpPeriod))
	uri := "otpauth://totp/" + url.PathEscape(label+":"+u.Email) + "?" + v.Encode()
	return TOTPSetup{FactorID: id, Secret: enc, URI: uri}, nil
}

// otpIssuer names this Hayat in the authenticator app: its host, so two
// deployments don't look alike.
func (s *Service) otpIssuer() string {
	if u, err := url.Parse(s.issuer); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "Hayat"
}

// checkFactorRoom caps how many factors one account holds.
func (s *Service) checkFactorRoom(ctx context.Context, tx pgx.Tx, u User) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_factors WHERE region = $1 AND user_id = $2 AND confirmed`, u.Region, u.ID).Scan(&n); err != nil {
		return err
	}
	if n >= 20 {
		return ErrTooManyFactors
	}
	return nil
}

// ConfirmTOTP finishes setting up an authenticator app with a code from it.
// first reports whether it is the person's first factor: the caller then
// hands out recovery codes.
func (s *Service) ConfirmTOTP(ctx context.Context, userID, factorID uuid.UUID, code, actor string) (first bool, err error) {
	if s.sec.Box == nil {
		return false, ErrNoBox
	}
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		var sealed []byte
		if err := tx.QueryRow(ctx, `SELECT secret_sealed FROM user_factors
			WHERE region = $1 AND id = $2 AND user_id = $3 AND kind = 'totp' AND NOT confirmed FOR UPDATE`,
			u.Region, factorID, userID).Scan(&sealed); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errs.New("this setup has expired: start again")
			}
			return err
		}
		secret, err := s.sec.Box.Open(sealed)
		if err != nil {
			return err
		}
		step, ok := matchTOTP(secret, code, time.Now(), 0)
		if !ok {
			return ErrBadCode
		}
		var others int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_factors WHERE region = $1 AND user_id = $2 AND confirmed`, u.Region, userID).Scan(&others); err != nil {
			return err
		}
		first = others == 0
		if _, err := tx.Exec(ctx, `UPDATE user_factors SET confirmed = true, last_step = $3, created_at = now()
			WHERE region = $1 AND id = $2`, u.Region, factorID, step); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, u.Region, actor, "user.factor_added", "user:"+userID.String(), map[string]any{"kind": FactorTOTP}); err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "An authenticator app was added to your Hayat account",
			fmt.Sprintf("Hi %s,\n\nSigning in now asks for a code from your authenticator app.\n\nIf this wasn't you, reset your password now: %s/forgot", u.DisplayName(), s.issuer))
	})
	return first, err
}

// VerifyTOTP checks a code from one of the person's authenticator apps. A
// code is accepted once: replaying it, even within its 30 seconds, fails.
func (s *Service) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	if s.sec.Box == nil {
		return ErrNoBox
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return ErrBadCode
	}
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, secret_sealed, last_step FROM user_factors
			WHERE region = $1 AND user_id = $2 AND kind = 'totp' AND confirmed FOR UPDATE`, region, userID)
		if err != nil {
			return err
		}
		type app struct {
			id     uuid.UUID
			sealed []byte
			last   int64
		}
		var apps []app
		for rows.Next() {
			var a app
			if err := rows.Scan(&a.id, &a.sealed, &a.last); err != nil {
				rows.Close()
				return err
			}
			apps = append(apps, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, a := range apps {
			secret, err := s.sec.Box.Open(a.sealed)
			if err != nil {
				return err
			}
			if step, ok := matchTOTP(secret, code, time.Now(), a.last); ok {
				_, err := tx.Exec(ctx, `UPDATE user_factors SET last_step = $3, last_used_at = now() WHERE region = $1 AND id = $2`, region, a.id, step)
				return err
			}
		}
		return ErrBadCode
	})
}

// matchTOTP finds the step a code belongs to, within the skew and newer than
// after.
func matchTOTP(secret []byte, code string, now time.Time, after int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		step := cur + d
		if step <= after {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// totpCode is HOTP (RFC 4226) at a time step.
func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, n%1_000_000)
}

// TOTPCode is the code an authenticator app would show now. Tests use it.
func TOTPCode(secret string, at time.Time) string {
	raw, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	return totpCode(raw, at.Unix()/totpPeriod)
}

// ---------- Recovery codes ----------

// recoveryAlphabet leaves out characters people mix up (0/o, 1/l/i).
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRecoveryCodes replaces the person's recovery codes with a fresh set and
// returns them, the one time they are shown.
func (s *Service) NewRecoveryCodes(ctx context.Context, userID uuid.UUID, actor string) ([]string, error) {
	codes := make([]string, recoveryCodeCount)
	for i := range codes {
		codes[i] = newRecoveryCode()
	}
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM recovery_codes WHERE region = $1 AND user_id = $2`, region, userID); err != nil {
			return err
		}
		for _, c := range codes {
			if _, err := tx.Exec(ctx, `INSERT INTO recovery_codes (region, id, user_id, code_hash) VALUES ($1, $2, $3, $4)`,
				region, store.NewID(), userID, crypto.HashToken(normalizeRecovery(c))); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, region, actor, "user.recovery_codes_created", "user:"+userID.String(), nil)
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func newRecoveryCode() string {
	// Bytes past the last whole multiple of the alphabet are dropped, so
	// every character is equally likely.
	limit := byte(256 - 256%len(recoveryAlphabet))
	out := make([]byte, 0, 11)
	var one [1]byte
	for len(out) < 11 {
		if len(out) == 5 {
			out = append(out, '-')
			continue
		}
		if _, err := rand.Read(one[:]); err != nil {
			panic(err)
		}
		if one[0] < limit {
			out = append(out, recoveryAlphabet[int(one[0])%len(recoveryAlphabet)])
		}
	}
	return string(out)
}

func normalizeRecovery(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

// RecoveryCodesLeft counts the unused codes.
func (s *Service) RecoveryCodesLeft(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.store.Pool.QueryRow(ctx, `SELECT count(*) FROM recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&n)
	return n, err
}

// UseRecoveryCode spends one recovery code in place of a second factor.
func (s *Service) UseRecoveryCode(ctx context.Context, userID uuid.UUID, code string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE recovery_codes SET used_at = now()
			WHERE region = $1 AND user_id = $2 AND code_hash = $3 AND used_at IS NULL`, u.Region, userID, crypto.HashToken(normalizeRecovery(code)))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errs.New("that recovery code isn't right or was already used")
		}
		if err := audit.Write(ctx, tx, u.Region, audit.UserActor(userID), "user.recovery_code_used", "user:"+userID.String(), nil); err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "A recovery code was used to sign in to Hayat",
			fmt.Sprintf("Hi %s,\n\nSomeone just signed in to your account with a recovery code. Each code works once; make a fresh set from Account → Two-step verification if you are running low.\n\nIf this wasn't you, reset your password now: %s/forgot", u.DisplayName(), s.issuer))
	})
}
