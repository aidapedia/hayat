// Package crypto holds the few primitives Hayat builds on: password hashing,
// opaque tokens, and authenticated encryption of secrets at rest.
//
// Nothing here is invented. Passwords use argon2id, tokens are 32 random bytes,
// stored secrets use AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters: OWASP's baseline (19 MiB, 2 passes) raised to 64 MiB,
// 3 passes. Stored with every hash, so they can be raised later without
// invalidating existing passwords.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword returns a PHC-formatted argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// LegacySHA256Prefix marks a password hash imported from another system that
// stored an unsalted SHA-256 hex digest (tabletsgo before Hayat). It verifies,
// so the people who had it can still sign in, and is replaced with argon2id the
// first time they do (NeedsRehash). Nothing in Hayat ever writes one.
const LegacySHA256Prefix = "$sha256$"

// NeedsRehash reports whether a hash that just verified should be replaced
// with a fresh argon2id one.
func NeedsRehash(hash string) bool { return strings.HasPrefix(hash, LegacySHA256Prefix) }

// VerifyPassword reports whether password matches hash. An empty hash never
// matches: it means the account has no password (it signs in through a provider).
func VerifyPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	if hex, ok := strings.CutPrefix(hash, LegacySHA256Prefix); ok {
		sum := sha256.Sum256([]byte(password))
		return subtle.ConstantTimeCompare([]byte(fmt.Sprintf("%x", sum)), []byte(strings.ToLower(hex))) == 1
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is verified against when an account doesn't exist, so a sign-in for
// an unknown email costs the same time as one for a known email.
var dummyHash, _ = HashPassword("hayat-timing-equaliser")

// BurnPasswordTime spends the time one VerifyPassword takes.
func BurnPasswordTime(password string) { VerifyPassword(dummyHash, password) }

var b64 = base64.RawStdEncoding

// NewToken returns a URL-safe random token of 32 bytes.
func NewToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // the system's randomness source failing is not recoverable
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// HashToken is how opaque tokens are stored: only their SHA-256, so a database
// read never yields a usable session, refresh token or email link.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Box encrypts and decrypts small secrets with a 32-byte master key.
type Box struct{ aead cipher.AEAD }

func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("crypto: master key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal returns nonce || ciphertext.
func (b *Box) Seal(plaintext []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil)
}

func (b *Box) Open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("crypto: sealed value too short")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], nil)
}
