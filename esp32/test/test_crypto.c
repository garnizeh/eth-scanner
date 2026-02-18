#include "unity.h"
#include "secp256k1.h"
#include "ecdsa.h"
#include <string.h>

void test_crypto_secp256k1_point_multiplication(void)
{
    // Standard test vector for secp256k1
    // Private Key: 0x01 (all zeros except the last byte)
    uint8_t priv_key[32] = {0};
    priv_key[31] = 0x01;

    uint8_t pub_key[65];
    // trezor-crypto ecdsa_get_public_key65 calculates the uncompressed 65-byte public key
    ecdsa_get_public_key65(&secp256k1, priv_key, pub_key);

    // Uncompressed public key should start with 0x04
    TEST_ASSERT_EQUAL_UINT8(0x04, pub_key[0]);

    // Expected X coordinate for priv_key = 1 (this is the generator G's X coordinate)
    uint8_t expected_x[] = {
        0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac,
        0x55, 0xa0, 0x62, 0x95, 0xce, 0x87, 0x0b, 0x07,
        0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9,
        0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98};

    // Expected Y coordinate for priv_key = 1 (this is the generator G's Y coordinate)
    uint8_t expected_y[] = {
        0x48, 0x3a, 0xda, 0x77, 0x26, 0xa3, 0xc4, 0x65,
        0x5d, 0xa4, 0xfb, 0xfc, 0x0e, 0x11, 0x08, 0xa8,
        0xfd, 0x17, 0xb4, 0x48, 0xa6, 0x85, 0x54, 0x19,
        0x9c, 0x47, 0xd0, 0x8f, 0xfb, 0x10, 0xd4, 0xb8};

    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected_x, &pub_key[1], 32);
    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected_y, &pub_key[33], 32);
}
