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
 * Derives the Ethereum address from a 32-byte private key.
 *
 * @param priv_key 32-byte private key.
 * @param address  Pointer to the 20-byte output buffer for the Ethereum address.
 */
void derive_eth_address(const uint8_t *priv_key, uint8_t *address);

/**
 * @brief Optimally updates the 4-byte nonce at the end of a 32-byte private key.
 *
 * This function performs direct byte manipulation to avoid expensive sprintf/memcpy.
 * The nonce is placed at offset 28 in little-endian format.
 *
 * @param buffer 32-byte private key buffer.
 * @param nonce  4-byte nonce to set.
 */
static inline void update_nonce_in_buffer(uint8_t *buffer, uint32_t nonce)
{
    buffer[28] = (uint8_t)(nonce & 0xFF);
    buffer[29] = (uint8_t)((nonce >> 8) & 0xFF);
    buffer[30] = (uint8_t)((nonce >> 16) & 0xFF);
    buffer[31] = (uint8_t)((nonce >> 24) & 0xFF);
}

#endif // ETH_CRYPTO_H
