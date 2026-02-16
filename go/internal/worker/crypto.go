package worker

import (
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
