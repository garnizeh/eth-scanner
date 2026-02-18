#include "esp_timer.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "eth_crypto.h"
#include <string.h>
#include <stdint.h>

#define BENCHMARK_ITERATIONS 100 // Reduced iterations for real crypto

static const char *TAG = "benchmark";

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

        // Feed watchdog periodically (every 10 iterations)
        if (i > 0 && (i % 10) == 0)
        {
            vTaskDelay(pdMS_TO_TICKS(1)); // Yield for at least 1 tick
        }
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
