package worker

import (
	"encoding/hex"
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

// TestDeriveEthereumAddress_TestVector uses a known private key -> address vector
// from common Ethereum examples to verify deterministic correctness.
func TestDeriveEthereumAddress_TestVector(t *testing.T) {
	t.Parallel()

	// Private key (hex): 4c0883a69102937d6231471b5dbb6204fe5129617082799a3f1c6f3b6e0f7f4a
	// Compute expected address using the go-ethereum library directly to ensure
	// deterministic agreement with the library functions used by the implementation.
	privHex := "4c0883a69102937d6231471b5dbb6204fe5129617082799a3f1c6f3b6e0f7f4a"
	var privArr [32]byte
	b, err := hex.DecodeString(privHex)
	if err != nil {
		t.Fatalf("failed to decode priv hex: %v", err)
	}
	copy(privArr[:], b)

	// Derive expected address via go-ethereum directly
	pk, err := crypto.ToECDSA(privArr[:])
	if err != nil {
		t.Fatalf("crypto.ToECDSA failed: %v", err)
	}
	want := crypto.PubkeyToAddress(pk.PublicKey)

	got, err := DeriveEthereumAddress(privArr)
	if err != nil {
		t.Fatalf("DeriveEthereumAddress returned error: %v", err)
	}
	if got != want {
		t.Fatalf("address mismatch: got %s, want %s", got.Hex(), want.Hex())
	}
}
