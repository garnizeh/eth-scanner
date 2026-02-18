#ifndef ETH_CRYPTO_H
#define ETH_CRYPTO_H

#include <stdint.h>
#include <stddef.h>

/**
 * Calculates the Keccak-256 hash of the input data.
 *
 * @param input Pointer to the input data.
 * @param len   Length of the input data in bytes.
 * @param output Pointer to the 32-byte output buffer where the hash will be stored.
 */
void keccak256(const uint8_t *input, size_t len, uint8_t *output);

/**
 * Derives the Ethereum address from an uncompressed 65-byte public key.
 * (Note: This might be implemented in a future task if not needed here.)
 *
 * @param pub_key Uncompressed public key (65 bytes, starting with 0x04).
 * @param address Pointer to the 20-byte output buffer for the Ethereum address.
 */
void derive_eth_address(const uint8_t *pub_key, uint8_t *address);

#endif // ETH_CRYPTO_H
