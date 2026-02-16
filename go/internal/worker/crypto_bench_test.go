package worker

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// BenchmarkDeriveEthereumAddress measures the cost of address derivation from a
// 32-byte private key using DeriveEthereumAddress.
func BenchmarkDeriveEthereumAddress(b *testing.B) {
	// Prepare a valid private key bytes once.
	key, err := crypto.GenerateKey()
	if err != nil {
		b.Fatalf("failed to generate key: %v", err)
	}
	privBytes := crypto.FromECDSA(key)
	var privArr [32]byte
	copy(privArr[:], privBytes[:32])

	b.ReportAllocs()

	for b.Loop() {
		if _, err := DeriveEthereumAddress(privArr); err != nil {
			b.Fatalf("DeriveEthereumAddress failed: %v", err)
		}
	}
}

// BenchmarkDeriveEthereumAddressParallel measures throughput under parallel
// execution using RunParallel.
func BenchmarkDeriveEthereumAddressParallel(b *testing.B) {
	key, err := crypto.GenerateKey()
	if err != nil {
		b.Fatalf("failed to generate key: %v", err)
	}
	privBytes := crypto.FromECDSA(key)
	var privArr [32]byte
	copy(privArr[:], privBytes[:32])

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := DeriveEthereumAddress(privArr); err != nil {
				b.Fatalf("DeriveEthereumAddress failed: %v", err)
			}
		}
	})
}

func BenchmarkConstructPrivateKey(b *testing.B) {
	var prefix [28]byte
	for i := range prefix {
		prefix[i] = byte(i)
	}
	nonce := uint32(0x12345678)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ConstructPrivateKey(prefix, nonce)
	}
}
