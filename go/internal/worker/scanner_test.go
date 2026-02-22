package worker

import (
	"context"
	"encoding/binary"
	"runtime"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestScanRange_NoMatch(t *testing.T) {
	t.Parallel()

	var prefix [28]byte
	for i := range 28 {
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
	got, err := ScanRange(ctx, job, []common.Address{commonAddressZero()})
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
	nonce := binary.BigEndian.Uint32(privBytes[28:32])

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
	got, err := ScanRange(ctx, job, []common.Address{expectedAddr})
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
	nonce := binary.BigEndian.Uint32(privBytes[28:32])

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
	got, err := ScanRange(ctx, job, []common.Address{expectedAddr})
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
	for i := range 28 {
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

	got, err := ScanRange(ctx, job, []common.Address{commonAddressZero()})
	if err == nil {
		t.Fatalf("expected context error, got nil and result %v", got)
	}
}

func TestScanRange_MultipleTargets(t *testing.T) {
	t.Parallel()

	// Target 1
	k1, _ := crypto.GenerateKey()
	p1 := crypto.FromECDSA(k1)
	var prefix [28]byte
	copy(prefix[:], p1[:28])
	n1 := binary.BigEndian.Uint32(p1[28:32])
	a1, _ := DeriveEthereumAddressFast([32]byte(p1), crypto.NewKeccakState(), &[64]byte{}, &[32]byte{})

	// Target 2 (different nonce, same prefix)
	n2 := n1 + 10
	var k2Full [32]byte
	copy(k2Full[:28], prefix[:])
	binary.BigEndian.PutUint32(k2Full[28:], n2)
	a2, _ := DeriveEthereumAddressFast(k2Full, crypto.NewKeccakState(), &[64]byte{}, &[32]byte{})

	job := Job{
		Prefix28:   prefix,
		NonceStart: n1 - 5,
		NonceEnd:   n2 + 5,
	}

	targets := []common.Address{commonAddressZero(), a1, a2}

	// Should find a1 first if we start from n1-5
	got, err := ScanRange(context.Background(), job, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Nonce != n1 {
		t.Fatalf("expected to find n1 (%d), got %v", n1, got)
	}

	// If we start from n1+1, it should find a2
	job.NonceStart = n1 + 1
	got, err = ScanRange(context.Background(), job, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Nonce != n2 {
		t.Fatalf("expected to find n2 (%d), got %v", n2, got)
	}
}

// commonAddressZero returns the zero address for convenience.
func commonAddressZero() (a common.Address) { return }

func TestScanRangeParallel_Match(t *testing.T) {
	t.Parallel()

	// Generate a real key
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privBytes := crypto.FromECDSA(key)

	var prefix [28]byte
	copy(prefix[:], privBytes[:28])
	nonce := binary.BigEndian.Uint32(privBytes[28:32])

	// Narrow range that definitely includes the nonce
	job := Job{
		ID:         5,
		Prefix28:   prefix,
		NonceStart: nonce - 100,
		NonceEnd:   nonce + 100,
	}

	var expectedKey [32]byte
	copy(expectedKey[:], privBytes[:32])
	expectedAddr, err := DeriveEthereumAddress(expectedKey)
	if err != nil {
		t.Fatalf("DeriveEthereumAddress failed: %v", err)
	}

	got, err := ScanRangeParallel(context.Background(), job, []common.Address{expectedAddr}, nil, runtime.NumCPU())
	if err != nil {
		t.Fatalf("ScanRangeParallel failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected result, got nil")
	}
	if got.Nonce != nonce {
		t.Errorf("nonce mismatch: got %d, want %d", got.Nonce, nonce)
	}
}

func TestScanRangeParallel_NoMatch(t *testing.T) {
	t.Parallel()

	job := Job{
		ID:         6,
		Prefix28:   [28]byte{1, 2, 3},
		NonceStart: 0,
		NonceEnd:   1000,
	}

	got, err := ScanRangeParallel(context.Background(), job, []common.Address{commonAddressZero()}, nil, runtime.NumCPU())
	if err != nil {
		t.Fatalf("ScanRangeParallel failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result, got %+v", got)
	}
}

func TestScanRangeParallel_Cancellation(t *testing.T) {
	t.Parallel()

	job := Job{
		ID:         7,
		Prefix28:   [28]byte{9, 9, 9},
		NonceStart: 0,
		NonceEnd:   1000000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := ScanRangeParallel(ctx, job, []common.Address{commonAddressZero()}, nil, runtime.NumCPU())
	if err == nil {
		t.Fatal("expected error due to timeout, got nil")
	}
}

func TestNonceBytesFromUint32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    uint32
		want [4]byte
	}{
		{name: "zero", n: 0, want: [4]byte{0, 0, 0, 0}},
		{name: "one", n: 1, want: [4]byte{0, 0, 0, 1}},
		{name: "max", n: 0xFFFFFFFF, want: [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{name: "pattern", n: 0x12345678, want: [4]byte{0x12, 0x34, 0x56, 0x78}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nonceBytesFromUint32(tt.n)
			if got != tt.want {
				t.Fatalf("bytes mismatch for %s: got %x want %x", tt.name, got, tt.want)
			}
		})
	}
}
