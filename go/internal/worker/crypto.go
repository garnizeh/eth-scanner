package worker

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// DeriveEthereumAddress derives the Ethereum address for a 32-byte private key.
// Returns an error if the private key is invalid.
func DeriveEthereumAddress(privateKey [32]byte) (common.Address, error) {
	pk, err := crypto.ToECDSA(privateKey[:])
	if err != nil {
		return common.Address{}, fmt.Errorf("invalid private key: %w", err)
	}
	return crypto.PubkeyToAddress(pk.PublicKey), nil
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
