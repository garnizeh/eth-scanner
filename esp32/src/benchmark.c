#include "esp_timer.h"
#include "esp_log.h"
#include <string.h>
#include <stdint.h>

#define BENCHMARK_ITERATIONS 10000

static const char *TAG = "benchmark";

/**
 * @brief Derive Ethereum address from private key.
 * This is a stub for benchmarking. The full implementation will be in P08.
 * It does some basic computations to simulate a crypto operation.
 */
void __attribute__((weak)) derive_eth_address(const uint8_t *privkey, uint8_t *address)
{
    uint8_t hash = 0;
    for (int i = 0; i < 32; i++)
    {
        hash ^= privkey[i] + i;
    }
    // Simulate some "work" that is roughly proportional to what we'd expect
    // but lightweight for now so tests aren't too slow.
    // Real secp256k1 + keccak is much heavier.
    for (volatile int i = 0; i < 1000; i++)
    {
        hash += (hash * 31) + 17;
    }
    memset(address, hash, 20);
}

uint32_t benchmark_key_generation(void)
{
    uint8_t privkey[32] = {0};
    uint8_t address[20];
    uint32_t nonce = 0;

    ESP_LOGI(TAG, "Starting benchmark (%d iterations)...", BENCHMARK_ITERATIONS);

    // Warm-up (exclude from measurement)
    for (int i = 0; i < 100; i++)
    {
        derive_eth_address(privkey, address);
    }

    // Benchmark loop
    int64_t start = esp_timer_get_time(); // microseconds

    for (int i = 0; i < BENCHMARK_ITERATIONS; i++)
    {
        // Simulate nonce increment (last 4 bytes)
        memcpy(&privkey[28], &nonce, sizeof(nonce));
        derive_eth_address(privkey, address);
        nonce++;
    }

    int64_t end = esp_timer_get_time();
    int64_t elapsed_us = end - start;

    // Calculate keys per second
    double elapsed_sec = elapsed_us / 1000000.0;
    double keys_per_sec = BENCHMARK_ITERATIONS / elapsed_sec;

    ESP_LOGI(TAG, "Benchmark complete: %.2f keys/sec (%.2f ms total)",
             keys_per_sec, elapsed_us / 1000.0);

    return (uint32_t)keys_per_sec;
}
