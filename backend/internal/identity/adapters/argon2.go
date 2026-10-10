// Package adapters contains the infrastructure implementations of the
// identity ports: argon2id hashing, JWT access tokens, PostgreSQL repositories
// and the HTTP handlers.
package adapters

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2idHasher hashes passwords with argon2id and a PHC-style encoding.
type Argon2idHasher struct {
	Time    uint32
	Memory  uint32 // KiB
	Threads uint8
	KeyLen  uint32
}

// DefaultArgon2idHasher uses OWASP-recommended minimum parameters.
func DefaultArgon2idHasher() Argon2idHasher {
	return Argon2idHasher{Time: 2, Memory: 19 * 1024, Threads: 1, KeyLen: 32}
}

// Hash implements app.PasswordHasher.
func (h Argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.Time, h.Memory, h.Threads, h.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.Memory, h.Time, h.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify implements app.PasswordHasher. Malformed hashes never verify.
func (h Argon2idHasher) Verify(password, encoded string) bool {
	p, salt, want, err := decodeArgon2(encoded)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func decodeArgon2(encoded string) (Argon2idHasher, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2idHasher{}, nil, nil, errors.New("bad format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Argon2idHasher{}, nil, nil, errors.New("bad version")
	}
	var p Argon2idHasher
	var threads uint
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &threads); err != nil {
		return Argon2idHasher{}, nil, nil, errors.New("bad params")
	}
	p.Threads = uint8(threads)
	if p.Memory > 256*1024 || p.Time > 10 || p.Threads == 0 { // refuse DoS-sized params
		return Argon2idHasher{}, nil, nil, errors.New("params out of range")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2idHasher{}, nil, nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Argon2idHasher{}, nil, nil, errors.New("bad key")
	}
	return p, salt, key, nil
}
