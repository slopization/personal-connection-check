package auth

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
	"io"
	"strings"
)

type Params struct {
	Memory, Iterations    uint32
	Parallelism           uint8
	SaltLength, KeyLength uint32
}

var Default = Params{65536, 3, 2, 16, 32}

const (
	maxMemory      uint32 = 262144
	maxIterations  uint32 = 10
	maxParallelism uint8  = 8
)

func parse(encoded string) (Params, []byte, []byte, bool) {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" || p[2] != "v=19" {
		return Params{}, nil, nil, false
	}
	var x Params
	if _, e := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &x.Memory, &x.Iterations, &x.Parallelism); e != nil || x.Memory < 8192 || x.Memory > maxMemory || x.Iterations < 1 || x.Iterations > maxIterations || x.Parallelism < 1 || x.Parallelism > maxParallelism {
		return Params{}, nil, nil, false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil || len(salt) < 16 {
		return Params{}, nil, nil, false
	}
	want, e := base64.RawStdEncoding.DecodeString(p[5])
	if e != nil || len(want) < 16 || len(want) > 64 {
		return Params{}, nil, nil, false
	}
	return x, salt, want, true
}
func ValidatePHC(encoded string) error {
	_, _, _, ok := parse(encoded)
	if !ok {
		return fmt.Errorf("invalid password hash parameters")
	}
	return nil
}
func Hash(password []byte) (string, error) {
	salt := make([]byte, Default.SaltLength)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	k := argon2.IDKey(password, salt, Default.Iterations, Default.Memory, Default.Parallelism, Default.KeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", Default.Memory, Default.Iterations, Default.Parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k)), nil
}
func Verify(encoded string, password []byte) bool {
	x, salt, want, ok := parse(encoded)
	if !ok {
		return false
	}
	got := argon2.IDKey(password, salt, x.Iterations, x.Memory, x.Parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}
func ReadPassword(r io.Reader) ([]byte, error) {
	b, e := bufio.NewReader(r).ReadString('\n')
	return []byte(strings.TrimRight(b, "\r\n")), e
}
