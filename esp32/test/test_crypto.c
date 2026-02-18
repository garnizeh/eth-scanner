#include "unity.h"
#include "secp256k1.h"
#include "ecdsa.h"
#include "eth_crypto.h"
#include <string.h>
#include <stdio.h>

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

void test_crypto_keccak256(void)
{
    // Test vector: empty string
    // keccak256("") = c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470
    uint8_t expected_empty[] = {
        0xc5, 0xd2, 0x46, 0x01, 0x86, 0xf7, 0x23, 0x3c, 0x92, 0x7e, 0x7d, 0xb2, 0xdc, 0xc7, 0x03, 0xc0,
        0xe5, 0x00, 0xb6, 0x53, 0xca, 0x82, 0x27, 0x3b, 0x7b, 0xfa, 0xd8, 0x04, 0x5d, 0x85, 0xa4, 0x70};
    uint8_t hash[32];

    keccak256((const uint8_t *)"", 0, hash);
    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected_empty, hash, 32);

    // Test vector: "The quick brown fox jumps over the lazy dog"
    // keccak256 = 4d741b6f1eb29cb2a9b9911c82f56fa8d73b04959d3d9d222895df6c0b28aa15
    const uint8_t input[] = "The quick brown fox jumps over the lazy dog";
    uint8_t expected_fox[] = {
        0x4d, 0x74, 0x1b, 0x6f, 0x1e, 0xb2, 0x9c, 0xb2, 0xa9, 0xb9, 0x91, 0x1c, 0x82, 0xf5, 0x6f, 0xa8,
        0xd7, 0x3b, 0x04, 0x95, 0x9d, 0x3d, 0x9d, 0x22, 0x28, 0x95, 0xdf, 0x6c, 0x0b, 0x28, 0xaa, 0x15};

    keccak256(input, sizeof(input) - 1, hash);
    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected_fox, hash, 32);
}

void test_crypto_derive_eth_address(void)
{
    // Private Key: 0x01
    // Uncompressed Public Key: 0x0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b116f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8
    uint8_t priv_key[32] = {0};
    priv_key[31] = 0x01;

    uint8_t address[20];
    derive_eth_address(priv_key, address);

    // Expected values from verified Go implementation
    uint8_t expected_address[] = {
        0x7e, 0x5f, 0x45, 0x52, 0x09, 0x1a, 0x69, 0x12, 0x5d, 0x5d,
        0xfc, 0xb7, 0xb8, 0xc2, 0x65, 0x90, 0x29, 0x39, 0x5b, 0xdf};

    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected_address, address, 20);
}
