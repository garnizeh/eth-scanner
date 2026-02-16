package worker

import (
	"encoding/binary"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// DeriveEthereumAddress derives the Ethereum address for a 32-byte private key.
// Returns an error if the private key is invalid.
// NOTE: This implementation is convenient but performs heap allocations.
// For hot loops, use the buffer-reusing variant or a specialized scanner.
func DeriveEthereumAddress(privateKey [32]byte) (common.Address, error) {
	pk, err := crypto.ToECDSA(privateKey[:])
	if err != nil {
		return common.Address{}, fmt.Errorf("invalid private key: %w", err)
	}
	return crypto.PubkeyToAddress(pk.PublicKey), nil
}

// DeriveEthereumAddressFast derives the Ethereum address into a provided buffer
// without heap allocations. It uses decred/dcrd/dcrec/secp256k1/v4 for
// allocation-free EC point multiplication and crypto.KeccakState for Keccak hashing.
//
// Optimization details:
//  1. Avoids *big.Int and ecdsa.PublicKey objects (zero-alloc EC path).
//  2. Reuses Keccak state to avoid hasher allocation.
//  3. Reuses public key and hash buffers to avoid slice allocations.
//  4. Scalar multiplication uses Non-Constant time variant as keys are public
//     knowledge in this specific brute-force educational context (speed focus).
func DeriveEthereumAddressFast(privateKey [32]byte, hasher crypto.KeccakState, pubBuf *[64]byte, hashBuf *[32]byte) (common.Address, error) {
	var scalar secp256k1.ModNScalar
	if overflow := scalar.SetBytes(&privateKey); overflow != 0 {
		return common.Address{}, fmt.Errorf("private key overflow")
	}
	if scalar.IsZero() {
		return common.Address{}, fmt.Errorf("invalid private key: zero")
	}

	// Calculate public key point: Q = d*G
	var point secp256k1.JacobianPoint
	secp256k1.ScalarBaseMultNonConst(&scalar, &point)
	point.ToAffine()

	// Extract X and Y coordinates (32 bytes each) into the uncompressed public key buffer.
	// We skip the 0x04 prefix byte as Ethereum hashes only the concatenated X|Y.
	point.X.Normalize()
	point.Y.Normalize()
	point.X.PutBytesUnchecked(pubBuf[0:32])
	point.Y.PutBytesUnchecked(pubBuf[32:64])

	// Hash the uncompressed public key (X|Y) using Keccak-256.
	hasher.Reset()
	_, _ = hasher.Write(pubBuf[:])
	hasher.Sum(hashBuf[:0])

	// The address is the last 20 bytes of the 32-byte Keccak-256 hash.
	var addr common.Address
	copy(addr[:], hashBuf[12:32])
	return addr, nil
}

// ConstructPrivateKey combines a 28-byte prefix with a 4-byte nonce to produce
// a deterministic 32-byte private key. The nonce is encoded using little-endian
// order so workers can partition the keyspace without heap allocations.
func ConstructPrivateKey(prefix28 [28]byte, nonce uint32) [32]byte {
	var key [32]byte
	copy(key[:28], prefix28[:])
	binary.LittleEndian.PutUint32(key[28:], nonce)
	return key
}
