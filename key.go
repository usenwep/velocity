package velocity

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	nwep "github.com/usenwep/nwep-go"
)

// LoadOrGenerateKeypair loads a keypair from a hex-encoded seed file at path.
// If the file does not exist, a new keypair is generated, the seed is written
// to path in hex encoding (with mode 0600), and the keypair is returned.
//
// The seed file must contain exactly 64 hex characters (32 bytes) optionally
// followed by whitespace. Leading and trailing whitespace is trimmed before
// decoding.
//
// This function returns a non-nil error if the file exists but cannot be read,
// if the hex seed is malformed, or if the generated key file cannot be written.
func LoadOrGenerateKeypair(path string) (*nwep.Keypair, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return KeypairFromHexSeed(strings.TrimSpace(string(data)))
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("velocity: read key file: %w", err)
	}
	kp, err := nwep.GenerateKeypair()
	if err != nil {
		return nil, fmt.Errorf("velocity: generate keypair: %w", err)
	}
	seed := kp.Seed()
	if err := os.WriteFile(path, []byte(hex.EncodeToString(seed[:])+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("velocity: write key file: %w", err)
	}
	return kp, nil
}

// KeypairFromHexSeed creates a keypair from a 64-character hex-encoded Ed25519
// seed. The string must decode to exactly 32 bytes. This function returns a
// non-nil error if s is not valid hex or has the wrong length.
func KeypairFromHexSeed(s string) (*nwep.Keypair, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("velocity: decode hex seed: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("velocity: seed must be 32 bytes, got %d", len(b))
	}
	var seed [32]byte
	copy(seed[:], b)
	return nwep.KeypairFromSeed(seed)
}

// KeypairFromEnv reads a hex-encoded Ed25519 seed from the environment variable
// named by envVar. Leading and trailing whitespace is trimmed before decoding.
// This function returns a non-nil error if the environment variable is not set
// or empty, or if the seed is malformed.
func KeypairFromEnv(envVar string) (*nwep.Keypair, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return nil, fmt.Errorf("velocity: env var %s not set", envVar)
	}
	return KeypairFromHexSeed(strings.TrimSpace(val))
}

// MustKeypair is a convenience wrapper that returns the keypair on success and
// panics if err is non-nil. It is intended for use in tests and initialization
// code where a keypair failure is unrecoverable:
//
//	kp := velocity.MustKeypair(nwep.GenerateKeypair())
func MustKeypair(kp *nwep.Keypair, err error) *nwep.Keypair {
	if err != nil {
		panic(err)
	}
	return kp
}
