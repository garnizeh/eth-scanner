package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestScanner_SingleWorkerUpdates(t *testing.T) {
	t.Parallel()

	s := NewScanner()
	s.UpdateInterval = 1 // update every nonce for test determinism

	job := Job{Prefix28: [28]byte{}, NonceStart: 0, NonceEnd: 100}
	ctx := context.Background()

	_, err := s.ScanRange(ctx, job, common.Address{})
	if err != nil {
		t.Fatalf("ScanRange returned error: %v", err)
	}

	if s.GetCurrentNonce() == 0 {
		t.Fatalf("expected current nonce > 0 after scan, got 0")
	}
}

func TestScanner_ConcurrentWorkers(t *testing.T) {
	t.Parallel()

	s := NewScanner()
	s.UpdateInterval = 1

	ranges := []Job{
		{Prefix28: [28]byte{}, NonceStart: 0, NonceEnd: 50},
		{Prefix28: [28]byte{}, NonceStart: 51, NonceEnd: 120},
		{Prefix28: [28]byte{}, NonceStart: 121, NonceEnd: 300},
	}

	ctx := context.Background()

	done := make(chan struct{})
	for _, j := range ranges {
		go func() {
			_, _ = s.ScanRange(ctx, j, common.Address{})
			done <- struct{}{}
		}()
	}

	// wait for all goroutines
	for range ranges {
		<-done
	}

	// Expect current nonce to be at least the highest end
	if s.GetCurrentNonce() < uint64(300) {
		t.Fatalf("expected current nonce >= 300, got %d", s.GetCurrentNonce())
	}
}

func TestScanner_CheckpointReaderDuringScan(t *testing.T) {
	s := NewScanner()
	s.UpdateInterval = 1

	job := Job{Prefix28: [28]byte{}, NonceStart: 0, NonceEnd: 10000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = s.ScanRange(ctx, job, common.Address{})
	}()

	<-started

	seen := false
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case <-deadline:
			if !seen {
				t.Fatal("checkpoint reader did not observe progress in time")
			}
			return
		default:
			if s.GetCurrentNonce() > 0 {
				cancel()
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
}

func TestScanner_DefaultUpdateInterval(t *testing.T) {
	t.Parallel()

	// Use zero-value Scanner to ensure ScanRange sets the default interval.
	s := &Scanner{}
	if s.UpdateInterval != 0 {
		t.Fatalf("expected zero-value UpdateInterval, got %d", s.UpdateInterval)
	}

	job := Job{Prefix28: [28]byte{}, NonceStart: 0, NonceEnd: 10}
	_, err := s.ScanRange(context.Background(), job, common.Address{})
	if err != nil {
		t.Fatalf("ScanRange returned error: %v", err)
	}

	if s.UpdateInterval != 1000 {
		t.Fatalf("expected UpdateInterval to be defaulted to 1000, got %d", s.UpdateInterval)
	}
}

func TestScanner_FoundResultUpdatesNonce(t *testing.T) {
	t.Parallel()

	s := NewScanner()
	s.UpdateInterval = 1000

	// pick a nonce inside the range that we will target
	var targetNonce uint32 = 37
	job := Job{Prefix28: [28]byte{}, NonceStart: 0, NonceEnd: 100}

	// construct the private key and target address for the chosen nonce
	key := ConstructPrivateKey(job.Prefix28, targetNonce)
	addr, err := DeriveEthereumAddress(key)
	if err != nil {
		t.Fatalf("failed to derive address: %v", err)
	}

	// run the scan targeting the known address
	res, err := s.ScanRange(context.Background(), job, addr)
	if err != nil {
		t.Fatalf("ScanRange returned error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected a result, got nil")
	}

	if res.Nonce != targetNonce {
		t.Fatalf("expected nonce %d, got %d", targetNonce, res.Nonce)
	}

	if s.GetCurrentNonce() < uint64(targetNonce) {
		t.Fatalf("expected scanner current nonce >= %d, got %d", targetNonce, s.GetCurrentNonce())
	}

	if res.PrivateKey != key {
		t.Fatalf("returned private key does not match expected key")
	}
}

func TestAddressEquals_DefaultStringComparison(t *testing.T) {
	t.Parallel()

	// derive an address for a known nonce
	key := ConstructPrivateKey([28]byte{}, 5)
	addr, err := DeriveEthereumAddress(key)
	if err != nil {
		t.Fatalf("failed to derive address: %v", err)
	}

	// default branch compares formatted values
	target := fmt.Sprintf("%v", addr)
	if !AddressEquals(addr, target) {
		t.Fatalf("expected AddressEquals to return true for matching string representation")
	}

	// different string should not match
	if AddressEquals(addr, "0xdeadbeef") {
		t.Fatalf("expected AddressEquals to return false for different target string")
	}
}
