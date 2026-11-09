package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// TokenPrefix marks Acksy API tokens so they are recognizable (and scannable
// by secret scanners).
const TokenPrefix = "ack_"

// displayPrefixLen is how many random characters are kept for display.
const displayPrefixLen = 8

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	return strings.ToLower(b32.EncodeToString(b))
}

// NewAPIToken returns a new token secret, its display prefix and its hash.
func NewAPIToken() (secret, prefix, hash string) {
	body := randomString(25) // 200 bits -> 40 base32 characters
	secret = TokenPrefix + body
	return secret, secret[:len(TokenPrefix)+displayPrefixLen], HashSecret(secret)
}

// LooksLikeAPIToken reports whether s has the API token shape.
func LooksLikeAPIToken(s string) bool {
	return strings.HasPrefix(s, TokenPrefix) && len(s) == len(TokenPrefix)+40
}

// NewSessionSecret returns a new session cookie value and its hash (the
// session ID stored in the database).
func NewSessionSecret() (secret, hash string) {
	secret = randomString(32)
	return secret, HashSecret(secret)
}

// HashSecret hashes a high-entropy secret for storage and lookup. Secrets
// are random, so a fast hash is sufficient (no brute-force risk).
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// NewWebhookSecret returns a random signing secret for a webhook.
func NewWebhookSecret() string { return "whsec_" + randomString(25) }
