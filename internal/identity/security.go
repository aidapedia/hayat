package identity

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // Have I Been Pwned indexes passwords by SHA-1; it is a lookup key here, not a hash we store
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
)

// Security is what account security needs beyond the database: the box TOTP
// secrets are sealed with, how long an unused session lives, and where new
// passwords are checked against known breaches. app wires it from config.
type Security struct {
	// Box seals TOTP secrets. Without it nobody can set up an authenticator app.
	Box *crypto.Box
	// IdleTimeout ends a session nobody used for this long; 0 is off.
	IdleTimeout time.Duration
	// Breached checks a new password against known breaches; nil skips it.
	Breached PasswordChecker
}

// SetSecurity installs the account-security settings. Call it once, before
// serving.
func (s *Service) SetSecurity(sec Security) { s.sec = sec }

// The methods that can stand in a session's amr (RFC 8176, plus Hayat's "fed").
const (
	AMRPassword = "pwd"
	AMROTP      = "otp" // a code from an authenticator app
	AMRPasskey  = "hwk" // a passkey: a key held by a device
	AMRMFA      = "mfa" // more than one factor, or a passkey the device unlocked
	AMRFed      = "fed" // another account: Google, GitHub, a company's provider
)

const (
	// Lockout: this many wrong passwords in a row lock the password for lockFor.
	maxFailedLogins = 10
	lockFor         = 15 * time.Minute

	// StepUpWindow is how recently someone must have proved it was them before
	// a sensitive change goes through without asking again.
	StepUpWindow = 10 * time.Minute
)

var (
	ErrBreachedPassword = errs.New("this password has appeared in a data breach, so attackers try it first: choose a different one")
	// ErrLocked reads like the rate limit on purpose: neither says whether the
	// address has an account.
	ErrLocked = errs.New("too many sign-in attempts: wait a few minutes and try again, or reset your password")
)

// checkNewPassword is every rule a password someone chooses must pass.
func (s *Service) checkNewPassword(ctx context.Context, p string) error {
	if utf8.RuneCountInString(p) < MinPasswordLength || len(p) > maxPasswordLength {
		return ErrWeakPassword
	}
	if s.sec.Breached == nil {
		return nil
	}
	breached, err := s.sec.Breached.Breached(ctx, p)
	if err != nil {
		// The lookup is a second opinion; an outage must not stop people
		// choosing a password.
		return nil
	}
	if breached {
		return ErrBreachedPassword
	}
	return nil
}

// PasswordChecker tells whether a password is known from a breach.
type PasswordChecker interface {
	Breached(ctx context.Context, password string) (bool, error)
}

// HIBP asks Have I Been Pwned's range API with k-anonymity: only the first
// five hex characters of the password's SHA-1 are sent, and the answer lists
// every suffix under them, padded so its size says nothing.
type HIBP struct {
	Client  *http.Client
	BaseURL string // https://api.pwnedpasswords.com; tests point it elsewhere
}

func NewHIBP() *HIBP {
	return &HIBP{Client: &http.Client{Timeout: 3 * time.Second}, BaseURL: "https://api.pwnedpasswords.com"}
}

func (h *HIBP) Breached(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // see the import
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := full[:5], full[5:]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+"/range/"+prefix, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "hayat")
	resp, err := h.Client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pwned passwords: %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		hash, count, ok := strings.Cut(line, ":")
		// Padding lines carry a count of 0.
		if ok && strings.EqualFold(hash, suffix) && strings.TrimSpace(count) != "0" {
			return true, nil
		}
	}
	return false, sc.Err()
}
