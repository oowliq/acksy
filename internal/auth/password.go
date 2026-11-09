// Package auth implements credentials: password hashing, API token and
// session secrets, and the authenticated Principal.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP baseline: m=19MiB, t=2, p=1).
type argonParams struct {
	memory  uint32 // KiB
	time    uint32
	threads uint8
	keyLen  uint32
	saltLen int
}

var defaultArgon = argonParams{memory: 19 * 1024, time: 2, threads: 1, keyLen: 32, saltLen: 16}

var b64 = base64.RawStdEncoding

// HashPassword returns an encoded argon2id hash in PHC string format.
func HashPassword(password string) (string, error) {
	p := defaultArgon
	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

var errBadHash = errors.New("auth: malformed password hash")

// CheckPassword reports whether password matches the encoded hash.
func CheckPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return false, errBadHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(want))) //nolint:gosec // len(want) is tiny
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash equalizes timing for unknown usernames.
var dummyHash, _ = HashPassword("acksy-timing-equalizer")

// BurnPasswordCheck spends the same time as a real password check.
func BurnPasswordCheck(password string) { _, _ = CheckPassword(dummyHash, password) }
