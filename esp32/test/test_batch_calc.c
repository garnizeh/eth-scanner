#include <unity.h>
#include "batch_calculator.h"
#include <esp_log.h>

void test_batch_calc_normal(void)
{
    // 200k keys/sec * 3600s = 720M raw.
    // Clamp to MAX_BATCH_SIZE (10M)
    uint32_t result = calculate_batch_size(200000, 3600);
    TEST_ASSERT_EQUAL_UINT32(10000000, result);
}

void test_batch_calc_small_throughput(void)
{
    // 10 keys/sec * 60s = 600 raw.
    // Clamp to MIN_BATCH_SIZE (10,000)
    uint32_t result = calculate_batch_size(10, 60);
    TEST_ASSERT_EQUAL_UINT32(10000, result);
}

void test_batch_calc_zero_throughput(void)
{
    // 0 throughput -> MIN_BATCH_SIZE
    uint32_t result = calculate_batch_size(0, 3600);
    TEST_ASSERT_EQUAL_UINT32(10000, result);
}

void test_batch_calc_zero_duration(void)
{
    // 0 duration -> 3600s fallback
    // 1000 keys/sec * 3600s * 0.95 = 3,420,000
    uint32_t result = calculate_batch_size(1000, 0);
    TEST_ASSERT_EQUAL_UINT32(3420000, result);
}

void test_batch_calc_mid_range(void)
{
    // 2 keys/sec * 3600s = 7200 raw
    // Clamped to MIN (10000)
    uint32_t result = calculate_batch_size(2, 3600);
    TEST_ASSERT_EQUAL_UINT32(10000, result);

    // 50 keys/sec * 600s (10 min) = 30k raw
    // * 0.95 = 28500
    uint32_t result2 = calculate_batch_size(50, 600);
    TEST_ASSERT_EQUAL_UINT32(28500, result2);
}

// Entry point for these tests (called from test_runner.c)
void run_batch_calc_tests(void)
{
    RUN_TEST(test_batch_calc_normal);
    RUN_TEST(test_batch_calc_small_throughput);
    RUN_TEST(test_batch_calc_zero_throughput);
    RUN_TEST(test_batch_calc_zero_duration);
    RUN_TEST(test_batch_calc_mid_range);
}
