#include "eth_crypto.h"
#include "sha3.h"
#include <string.h>

void keccak256(const uint8_t *input, size_t len, uint8_t *output)
{
    // trezor-crypto's keccak_256 function takes data, len and a result buffer.
    keccak_256(input, len, output);
}

void derive_eth_address(const uint8_t *pub_key, uint8_t *address)
{
    // Ethereum address is the last 20 bytes of Keccak-256(pub_key[1:65])
    // The pub_key is 65 bytes long (starts with 0x04 for uncompressed).
    // The hash is computed on the 64-byte part (everything but the 0x04 prefix byte).

    uint8_t hash[32];
    keccak256(pub_key + 1, 64, hash);

    // Copy the last 20 bytes of the hash to the address buffer
    memcpy(address, hash + 12, 20);
}
