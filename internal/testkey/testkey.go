// Package testkey is a software passkey for tests: it answers WebAuthn
// registration and sign-in options the way a phone or security key would
// (ES256, "none" attestation, the owner verified), so passkey flows are
// tested end to end with real messages.
package testkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

var b64 = base64.RawURLEncoding

// Key is one passkey held by a pretend device.
type Key struct {
	Origin string // the page's origin, e.g. http://auth.test
	ID     []byte
	UserID []byte
	priv   *ecdsa.PrivateKey
	count  uint32
}

func New(origin string) *Key {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &Key{Origin: origin, ID: id, priv: priv}
}

type options struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		RPID string `json:"rpId"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"publicKey"`
}

// Register answers navigator.credentials.create options with a new credential.
func (k *Key) Register(optionsJSON []byte) ([]byte, error) {
	var o options
	if err := json.Unmarshal(optionsJSON, &o); err != nil {
		return nil, err
	}
	uid, err := b64.DecodeString(o.PublicKey.User.ID)
	if err != nil {
		return nil, fmt.Errorf("user id: %w", err)
	}
	k.UserID = uid
	client, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": o.PublicKey.Challenge, "origin": k.Origin, "crossOrigin": false})

	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: pad32(k.priv.X.Bytes()), -3: pad32(k.priv.Y.Bytes())})
	if err != nil {
		return nil, err
	}
	auth := k.authData(o.PublicKey.RP.ID, 0x01|0x04|0x40) // user present, verified, attested data
	auth = append(auth, make([]byte, 16)...)              // AAGUID
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(k.ID)))
	auth = append(auth, k.ID...)
	auth = append(auth, cose...)
	att, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": auth})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id": b64.EncodeToString(k.ID), "rawId": b64.EncodeToString(k.ID), "type": "public-key",
		"response": map[string]any{"clientDataJSON": b64.EncodeToString(client), "attestationObject": b64.EncodeToString(att),
			"transports": []string{"internal"}},
		"clientExtensionResults": map[string]any{},
	})
}

// Sign answers navigator.credentials.get options.
func (k *Key) Sign(optionsJSON []byte) ([]byte, error) {
	var o options
	if err := json.Unmarshal(optionsJSON, &o); err != nil {
		return nil, err
	}
	client, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": o.PublicKey.Challenge, "origin": k.Origin, "crossOrigin": false})
	k.count++
	auth := k.authData(o.PublicKey.RPID, 0x01|0x04)
	sum := sha256.Sum256(client)
	digest := sha256.Sum256(append(append([]byte{}, auth...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id": b64.EncodeToString(k.ID), "rawId": b64.EncodeToString(k.ID), "type": "public-key",
		"response": map[string]any{"clientDataJSON": b64.EncodeToString(client), "authenticatorData": b64.EncodeToString(auth),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(k.UserID)},
		"clientExtensionResults": map[string]any{},
	})
}

func (k *Key) authData(rpID string, flags byte) []byte {
	h := sha256.Sum256([]byte(rpID))
	out := append([]byte{}, h[:]...)
	out = append(out, flags)
	return binary.BigEndian.AppendUint32(out, k.count)
}

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	return append(make([]byte, 32-len(b)), b...)
}
