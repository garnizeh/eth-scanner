set(srcs
    "address.c"
    "base32.c"
    "base58.c"
    "bignum.c"
    "bip32.c"
    "bip39.c"
    "blake256.c"
    "blake2b.c"
    "blake2s.c"
    "cash_addr.c"
    "curves.c"
    "ecdsa.c"
    "groestl.c"
    "hasher.c"
    "hmac.c"
    "hmac_drbg.c"
    "memzero.c"
    "nem.c"
    "nist256p1.c"
    "pbkdf2.c"
    "rand.c"
    "rand_esp32.c"
    "rc4.c"
    "rfc6979.c"
    "ripemd160.c"
    "script.c"
    "secp256k1.c"
    "segwit_addr.c"
    "sha2.c"
    "sha3.c"
    "shamir.c"
    "slip39.c"
)

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS "."
)

target_compile_definitions(${COMPONENT_LIB} PRIVATE 
    -DUSE_ETHEREUM=1
    -DUSE_KECCAK=1
    -DUSE_PRECOMPUTED_CP=1
    -DUSE_INVERSE_FAST=1
    -DRAND_PLATFORM_INDEPENDENT=1
)

target_compile_options(${COMPONENT_LIB} PRIVATE
    -Wno-error=array-parameter
    -Wno-array-parameter
)
