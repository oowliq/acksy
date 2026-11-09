package auth

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// VAPIDKeys is a Web Push application server key pair (RFC 8292): an ECDSA
// P-256 key, encoded as unpadded base64url — the private key as the raw
// 32-byte scalar, the public key as the 65-byte uncompressed point that
// browsers expect as applicationServerKey.
type VAPIDKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

// NewVAPIDKeys generates a fresh key pair.
func NewVAPIDKeys() (VAPIDKeys, error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return VAPIDKeys{}, fmt.Errorf("generate VAPID key: %w", err)
	}
	enc := base64.RawURLEncoding
	return VAPIDKeys{Public: enc.EncodeToString(k.PublicKey().Bytes()), Private: enc.EncodeToString(k.Bytes())}, nil
}

// Validate checks that the keys decode and belong together.
func (k VAPIDKeys) Validate() error {
	enc := base64.RawURLEncoding
	priv, err := enc.DecodeString(k.Private)
	if err != nil {
		return fmt.Errorf("VAPID private key: %w", err)
	}
	key, err := ecdh.P256().NewPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("VAPID private key: %w", err)
	}
	pub, err := enc.DecodeString(k.Public)
	if err != nil {
		return fmt.Errorf("VAPID public key: %w", err)
	}
	if string(key.PublicKey().Bytes()) != string(pub) {
		return fmt.Errorf("VAPID public key does not match the private key")
	}
	return nil
}
