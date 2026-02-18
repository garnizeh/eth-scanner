#include "eth_crypto.h"
#include "sha3.h"
#include "ecdsa.h"
#include "secp256k1.h"
#include <string.h>

void keccak256(const uint8_t *input, size_t len, uint8_t *output)
{
    // trezor-crypto's keccak_256 function takes data, len and a result buffer.
    keccak_256(input, len, output);
}

void derive_eth_address(const uint8_t *priv_key, uint8_t *address)
{
    // 1. Get the uncompressed 65-byte public key (starts with 0x04)
    uint8_t pub_key[65];
    ecdsa_get_public_key65(&secp256k1, priv_key, pub_key);

    // 2. Ethereum address is the last 20 bytes of Keccak-256(pub_key[1:65])
    // The hash is computed on the 64-byte part (everything but the 0x04 prefix byte).
    uint8_t hash[32];
    keccak256(pub_key + 1, 64, hash);

    // 3. Copy the last 20 bytes of the hash to the address buffer
    memcpy(address, hash + 12, 20);

    // Security: Zero the public key and hash after use
    memset(pub_key, 0, sizeof(pub_key));
    memset(hash, 0, sizeof(hash));
}
