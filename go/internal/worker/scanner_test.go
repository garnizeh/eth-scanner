package worker

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestScanRange_NoMatch(t *testing.T) {
	t.Parallel()

	var prefix [28]byte
	for i := range prefix {
		prefix[i] = byte(i + 1)
	}

	job := Job{
		ID:         1,
		Prefix28:   prefix,
		NonceStart: 0,
		NonceEnd:   99,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}

	ctx := context.Background()
	got, err := ScanRange(ctx, job, (commonAddressZero()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no result, got %+v", got)
	}
}

func TestScanRange_FindAtNonce(t *testing.T) {
	t.Parallel()

	// Generate a real key and split into prefix + nonce.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privBytes := crypto.FromECDSA(key)

	var prefix [28]byte
	copy(prefix[:], privBytes[:28])
	nonce := binary.LittleEndian.Uint32(privBytes[28:32])

	job := Job{
		ID:         2,
		Prefix28:   prefix,
		NonceStart: nonce - 1,
		NonceEnd:   nonce + 1,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}

	// compute expected target from the generated key
	var expectedKey [32]byte
	copy(expectedKey[:], privBytes[:32])
	expectedAddr, err := DeriveEthereumAddress(expectedKey)
	if err != nil {
		t.Fatalf("DeriveEthereumAddress failed: %v", err)
	}

	ctx := context.Background()
	got, err := ScanRange(ctx, job, expectedAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a result but got nil")
	}
	if got.Nonce != nonce {
		t.Fatalf("nonce mismatch: got %d want %d", got.Nonce, nonce)
	}
}

func TestScanRange_InclusiveEnd(t *testing.T) {
	t.Parallel()

	// Create key where nonce is at the end of the range.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privBytes := crypto.FromECDSA(key)
	var prefix [28]byte
	copy(prefix[:], privBytes[:28])
	nonce := binary.LittleEndian.Uint32(privBytes[28:32])

	job := Job{
		ID:         3,
		Prefix28:   prefix,
		NonceStart: nonce - 5,
		NonceEnd:   nonce,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}

	var expectedKey [32]byte
	copy(expectedKey[:], privBytes[:32])
	expectedAddr, err := DeriveEthereumAddress(expectedKey)
	if err != nil {
		t.Fatalf("DeriveEthereumAddress failed: %v", err)
	}

	ctx := context.Background()
	got, err := ScanRange(ctx, job, expectedAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a result at end of range but got nil")
	}
	if got.Nonce != nonce {
		t.Fatalf("nonce mismatch at end: got %d want %d", got.Nonce, nonce)
	}
}

func TestScanRange_ContextCancelled(t *testing.T) {
	t.Parallel()

	var prefix [28]byte
	for i := range prefix {
		prefix[i] = byte(i)
	}

	job := Job{
		ID:         4,
		Prefix28:   prefix,
		NonceStart: 0,
		NonceEnd:   100000,
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}

	ctx, cancel := context.WithCancel(context.Background())
	// cancel before calling ScanRange; first iteration checks ctx and should return
	cancel()

	got, err := ScanRange(ctx, job, commonAddressZero())
	if err == nil {
		t.Fatalf("expected context error, got nil and result %v", got)
	}
}

// commonAddressZero returns the zero address for convenience.
func commonAddressZero() (a common.Address) { return }

func TestNonceBytesFromUint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    uint32
		want [4]byte
	}{
		{name: "zero", n: 0, want: [4]byte{0, 0, 0, 0}},
		{name: "one", n: 1, want: [4]byte{1, 0, 0, 0}},
		{name: "max", n: 0xFFFFFFFF, want: [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{name: "pattern", n: 0x12345678, want: [4]byte{0x78, 0x56, 0x34, 0x12}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nonceBytesFromUint32(tt.n)
			if got != tt.want {
				t.Fatalf("bytes mismatch for %s: got %x want %x", tt.name, got, tt.want)
			}
		})
	}
}
