package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	hmail "github.com/aidapedia/hayat/internal/mail"
	"github.com/aidapedia/hayat/internal/store"
)

// Passkeys are WebAuthn credentials whose relying party is Hayat's issuer:
// they are made and used on the sign-in pages only, never on a service's or
// the console's origin. Every ceremony asks the device to verify its owner
// (fingerprint, face, PIN), so a passkey alone is two factors.

var (
	ErrPasskeyFailed = errs.New("that passkey didn't work: try again, or sign in another way")
	ErrNoPasskeys    = errs.New("passkeys aren't available on this server")
)

func newRelyingParty(issuer string) *webauthn.WebAuthn {
	u, err := url.Parse(issuer)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	w, err := webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "Hayat",
		RPOrigins:     []string{u.Scheme + "://" + u.Host},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute},
		},
	})
	if err != nil {
		return nil
	}
	return w
}

// passkeyUser is an account as the WebAuthn library sees it. Its handle is
// the account id: the 16 bytes of the UUID.
type passkeyUser struct {
	u     User
	creds []webauthn.Credential
}

func (p passkeyUser) WebAuthnID() []byte                         { return p.u.ID[:] }
func (p passkeyUser) WebAuthnName() string                       { return p.u.Email }
func (p passkeyUser) WebAuthnDisplayName() string                { return p.u.DisplayName() }
func (p passkeyUser) WebAuthnCredentials() []webauthn.Credential { return p.creds }

func (s *Service) loadPasskeyUser(ctx context.Context, db store.DB, u User) (passkeyUser, error) {
	rows, err := db.Query(ctx, `SELECT credential FROM user_factors WHERE region = $1 AND user_id = $2 AND kind = 'passkey' AND confirmed`, u.Region, u.ID)
	if err != nil {
		return passkeyUser{}, err
	}
	defer rows.Close()
	pu := passkeyUser{u: u}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return passkeyUser{}, err
		}
		var c webauthn.Credential
		if err := json.Unmarshal(raw, &c); err != nil {
			return passkeyUser{}, err
		}
		pu.creds = append(pu.creds, c)
	}
	return pu, rows.Err()
}

// PasskeysAvailable reports whether this server can be a passkey's relying
// party (its issuer has a host name).
func (s *Service) PasskeysAvailable() bool { return s.passkeys != nil }

// HasPasskey reports whether the person has a passkey to sign in or confirm with.
func (s *Service) HasPasskey(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_factors WHERE user_id = $1 AND kind = 'passkey' AND confirmed)`, userID).Scan(&ok)
	return ok, err
}

// BeginPasskeyRegistration returns the options for navigator.credentials.create
// and the ceremony's state, which the caller keeps until FinishPasskeyRegistration.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, u User) (options, state []byte, err error) {
	if s.passkeys == nil {
		return nil, nil, ErrNoPasskeys
	}
	pu, err := s.loadPasskeyUser(ctx, s.store.Pool, u)
	if err != nil {
		return nil, nil, err
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(pu.creds))
	for _, c := range pu.creds {
		exclude = append(exclude, c.Descriptor())
	}
	creation, session, err := s.passkeys.BeginRegistration(pu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude))
	if err != nil {
		return nil, nil, err
	}
	return marshalPair(creation, session)
}

// FinishPasskeyRegistration checks the device's answer and keeps the passkey.
// first reports whether it is the person's first factor (recovery codes next).
func (s *Service) FinishPasskeyRegistration(ctx context.Context, u User, state, response []byte, name, actor string) (first bool, err error) {
	if s.passkeys == nil {
		return false, ErrNoPasskeys
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(state, &session); err != nil {
		return false, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return false, ErrPasskeyFailed
	}
	pu, err := s.loadPasskeyUser(ctx, s.store.Pool, u)
	if err != nil {
		return false, err
	}
	cred, err := s.passkeys.CreateCredential(pu, session, parsed)
	if err != nil {
		return false, ErrPasskeyFailed
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return false, err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = "Passkey"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	id := store.NewID()
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.checkFactorRoom(ctx, tx, u); err != nil {
			return err
		}
		var others int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_factors WHERE region = $1 AND user_id = $2 AND confirmed`, u.Region, u.ID).Scan(&others); err != nil {
			return err
		}
		first = others == 0
		sp, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := sp.Exec(ctx, `INSERT INTO passkey_refs (credential_id, user_id, region, factor_id) VALUES ($1, $2, $3, $4)`,
			cred.ID, u.ID, u.Region, id); err != nil {
			_ = sp.Rollback(ctx)
			if store.IsUniqueViolation(err) {
				return errs.New("this passkey is already set up")
			}
			return err
		}
		if err := sp.Commit(ctx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_factors (region, id, user_id, kind, name, credential_id, credential, confirmed)
			VALUES ($1, $2, $3, 'passkey', $4, $5, $6::jsonb, true)`, u.Region, id, u.ID, name, cred.ID, string(raw)); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, u.Region, actor, "user.factor_added", "user:"+u.ID.String(), map[string]any{"kind": FactorPasskey}); err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "A passkey was added to your Hayat account",
			fmt.Sprintf("Hi %s,\n\nA passkey named %q was added to your account. You can sign in with it instead of a password.\n\nIf this wasn't you, reset your password now: %s/forgot", u.DisplayName(), name, s.issuer))
	})
	return first, err
}

// BeginPasskeyLogin returns the options for navigator.credentials.get. With
// no user it is "Sign in with a passkey": the device offers any passkey it
// holds for Hayat. With a user it confirms that person (a second factor, or
// confirming it's them before a sensitive change).
func (s *Service) BeginPasskeyLogin(ctx context.Context, u *User) (options, state []byte, err error) {
	if s.passkeys == nil {
		return nil, nil, ErrNoPasskeys
	}
	uv := webauthn.WithUserVerification(protocol.VerificationRequired)
	if u == nil {
		assertion, session, err := s.passkeys.BeginDiscoverableLogin(uv)
		if err != nil {
			return nil, nil, err
		}
		return marshalPair(assertion, session)
	}
	pu, err := s.loadPasskeyUser(ctx, s.store.Pool, *u)
	if err != nil {
		return nil, nil, err
	}
	if len(pu.creds) == 0 {
		return nil, nil, ErrPasskeyFailed
	}
	assertion, session, err := s.passkeys.BeginLogin(pu, uv)
	if err != nil {
		return nil, nil, err
	}
	return marshalPair(assertion, session)
}

// FinishPasskeyLogin checks the device's answer and returns whose passkey it
// was. want, when set, is the only account the answer may be for.
func (s *Service) FinishPasskeyLogin(ctx context.Context, state, response []byte, want *uuid.UUID) (User, error) {
	if s.passkeys == nil {
		return User{}, ErrNoPasskeys
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(state, &session); err != nil {
		return User{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return User{}, ErrPasskeyFailed
	}
	var owner User
	var cred *webauthn.Credential
	if want == nil {
		var who webauthn.User
		who, cred, err = s.passkeys.ValidatePasskeyLogin(func(rawID, handle []byte) (webauthn.User, error) {
			var ref struct {
				user   uuid.UUID
				region string
			}
			if err := s.store.Pool.QueryRow(ctx, `SELECT user_id, region FROM passkey_refs WHERE credential_id = $1`, rawID).
				Scan(&ref.user, &ref.region); err != nil {
				return nil, err
			}
			if !bytes.Equal(ref.user[:], handle) {
				return nil, errors.New("user handle does not match the passkey's account")
			}
			u, err := s.GetUser(ctx, ref.user)
			if err != nil {
				return nil, err
			}
			return s.loadPasskeyUser(ctx, s.store.Pool, u)
		}, session, parsed)
		if err == nil {
			owner = who.(passkeyUser).u
		}
	} else {
		var u User
		if u, err = s.GetUser(ctx, *want); err != nil {
			return User{}, err
		}
		var pu passkeyUser
		if pu, err = s.loadPasskeyUser(ctx, s.store.Pool, u); err != nil {
			return User{}, err
		}
		cred, err = s.passkeys.ValidateLogin(pu, session, parsed)
		owner = u
	}
	if err != nil {
		return User{}, ErrPasskeyFailed
	}
	if cred.Authenticator.CloneWarning {
		// The device's counter went backwards: two copies of one key exist.
		return User{}, ErrPasskeyFailed
	}
	if !cred.Flags.UserVerified {
		return User{}, ErrPasskeyFailed
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return User{}, err
	}
	if _, err := s.store.Pool.Exec(ctx, `UPDATE user_factors SET credential = $4::jsonb, last_used_at = now()
		WHERE region = $1 AND user_id = $2 AND credential_id = $3 AND kind = 'passkey'`, owner.Region, owner.ID, cred.ID, string(raw)); err != nil {
		return User{}, err
	}
	return owner, nil
}

func marshalPair(options any, session *webauthn.SessionData) ([]byte, []byte, error) {
	o, err := json.Marshal(options)
	if err != nil {
		return nil, nil, err
	}
	st, err := json.Marshal(session)
	if err != nil {
		return nil, nil, err
	}
	return o, st, nil
}
