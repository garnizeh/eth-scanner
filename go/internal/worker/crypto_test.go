package worker

import (
	"bytes"
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

func TestDeriveEthereumAddressFast_MatchesStandard(t *testing.T) {
	t.Parallel()

	// Generate a few random keys and verify they produce the same address.
	for i := range 10 {
		key, _ := crypto.GenerateKey()
		var privArr [32]byte
		copy(privArr[:], crypto.FromECDSA(key))

		want, err := DeriveEthereumAddress(privArr)
		if err != nil {
			t.Fatalf("DeriveEthereumAddress failed: %v", err)
		}

		hasher := crypto.NewKeccakState()
		var pubBuf [64]byte
		var hashBuf [32]byte
		got, err := DeriveEthereumAddressFast(privArr, hasher, &pubBuf, &hashBuf)
		if err != nil {
			t.Fatalf("DeriveEthereumAddressFast failed: %v", err)
		}

		if got != want {
			t.Fatalf("mismatch at key %d: got %s, want %s", i, got.Hex(), want.Hex())
		}
	}
}

func TestConstructPrivateKey(t *testing.T) {
	t.Parallel()

	var prefix [28]byte
	for i := range 28 {
		prefix[i] = byte(i + 1)
	}

	tests := []struct {
		name       string
		nonce      uint32
		wantSuffix [4]byte
	}{
		{name: "nonce=0", nonce: 0, wantSuffix: [4]byte{0, 0, 0, 0}},
		{name: "nonce=1", nonce: 1, wantSuffix: [4]byte{1, 0, 0, 0}},
		{name: "nonce=max", nonce: 0xFFFFFFFF, wantSuffix: [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{name: "nonce=little_endian", nonce: 0x12345678, wantSuffix: [4]byte{0x78, 0x56, 0x34, 0x12}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ConstructPrivateKey(prefix, tt.nonce)
			if !bytes.Equal(got[:28], prefix[:]) {
				t.Fatalf("prefix mismatch: got %x, want %x", got[:28], prefix)
			}
			if !bytes.Equal(got[28:], tt.wantSuffix[:]) {
				t.Fatalf("nonce bytes mismatch: got %x, want %x", got[28:], tt.wantSuffix)
			}
		})
	}
}

// TestDeriveEthereumAddress_Overflow verifies that a key above the secp256k1
// group order returns an error.
func TestDeriveEthereumAddress_Overflow(t *testing.T) {
	t.Parallel()

	// secp256k1 group order is:
	// FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141
	// We'll use all Fs to ensure overflow.
	var overflow [32]byte
	for i := range 32 {
		overflow[i] = 0xFF
	}

	_, err := DeriveEthereumAddress(overflow)
	if err == nil {
		t.Fatalf("expected error for overflow private key, got nil")
	}

	hasher := crypto.NewKeccakState()
	var pubBuf [64]byte
	var hashBuf [32]byte
	_, err = DeriveEthereumAddressFast(overflow, hasher, &pubBuf, &hashBuf)
	if err == nil {
		t.Fatalf("expected error for overflow private key (fast), got nil")
	}
}

// TestDeriveEthereumAddressFast_InvalidKey verifies that an invalid (zero) key
// is caught by the fast implementation.
func TestDeriveEthereumAddressFast_InvalidKey(t *testing.T) {
	t.Parallel()

	var zero [32]byte
	hasher := crypto.NewKeccakState()
	var pubBuf [64]byte
	var hashBuf [32]byte
	_, err := DeriveEthereumAddressFast(zero, hasher, &pubBuf, &hashBuf)
	if err == nil {
		t.Fatalf("expected error for zero private key (fast), got nil")
	}
}

func TestConstructPrivateKey_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("AllZeros", func(t *testing.T) {
		var prefix [28]byte
		got := ConstructPrivateKey(prefix, 0)
		var expected [32]byte
		if !bytes.Equal(got[:], expected[:]) {
			t.Fatalf("expected all zeros, got %x", got)
		}
	})

	t.Run("AllOnes", func(t *testing.T) {
		var prefix [28]byte
		for i := range 28 {
			prefix[i] = 0xFF
		}
		got := ConstructPrivateKey(prefix, 0xFFFFFFFF)
		var expected [32]byte
		for i := range 32 {
			expected[i] = 0xFF
		}
		if !bytes.Equal(got[:], expected[:]) {
			t.Fatalf("expected all ones, got %x", got)
		}
	})
}
