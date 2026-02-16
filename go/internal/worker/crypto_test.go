package worker

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// TestDeriveEthereumAddress_Success verifies that DeriveEthereumAddress
// returns the same address as go-ethereum's PubkeyToAddress for a generated key.
func TestDeriveEthereumAddress_Success(t *testing.T) {
	t.Parallel()

	// Generate a fresh ECDSA key for the test.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Build the 32-byte private key array used by the implementation.
	privBytes := crypto.FromECDSA(key)
	var privArr [32]byte
	copy(privArr[:], privBytes[:32])

	expected := crypto.PubkeyToAddress(key.PublicKey)

	addr, err := DeriveEthereumAddress(privArr)
	if err != nil {
		t.Fatalf("DeriveEthereumAddress returned error: %v", err)
	}
	if addr != expected {
		t.Fatalf("address mismatch: got %s, want %s", addr.Hex(), expected.Hex())
	}
}

// TestDeriveEthereumAddress_InvalidKey verifies that an invalid (zero) key
// returns an error rather than a (possibly incorrect) address.
func TestDeriveEthereumAddress_InvalidKey(t *testing.T) {
	t.Parallel()

	var zero [32]byte
	_, err := DeriveEthereumAddress(zero)
	if err == nil {
		t.Fatalf("expected error for invalid private key, got nil")
	}
}
