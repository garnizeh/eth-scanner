#include <unity.h>
#include "benchmark.h"
#include "esp_log.h"

static const char *TAG = "test_benchmark";

void test_benchmark_positive_throughput(void)
{
    ESP_LOGI(TAG, "Running throughput benchmark test...");
    uint32_t throughput = benchmark_key_generation();

    // Throughput should be positive
    TEST_ASSERT_TRUE(throughput > 0);
    ESP_LOGI(TAG, "Test passed: throughput = %lu keys/sec", (unsigned long)throughput);
}

void test_benchmark_repeatability(void)
{
    uint32_t t1 = benchmark_key_generation();
    uint32_t t2 = benchmark_key_generation();

    // They should be reasonably close (within 20%)
    uint32_t diff = (t1 > t2) ? (t1 - t2) : (t2 - t1);
    uint32_t threshold = t1 / 5; // 20%

    TEST_ASSERT_TRUE(diff < threshold);
}
