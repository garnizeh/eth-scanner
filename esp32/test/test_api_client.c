#include <unity.h>
#include "api_client.h"
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"
#include <string.h>

static const char *TAG = "test_api_client";

void test_api_lease_success()
{
    job_info_t job;
    esp_err_t err = api_lease_job("test-worker", 5000, &job);
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(42, job.job_id);
    TEST_ASSERT_EQUAL(1000, job.nonce_start);
    TEST_ASSERT_EQUAL(2000, job.nonce_end);
    TEST_ASSERT_EQUAL_STRING("0x742d35Cc6634C0532925a3b844Bc454e4438f44e", job.target_address);
    // Prefix 1..28
    uint8_t expected_prefix[28];
    for (int i = 0; i < 28; i++)
        expected_prefix[i] = i + 1;
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected_prefix, job.prefix_28, 28);
}

void test_api_checkpoint()
{
    esp_err_t err = api_checkpoint(42, "test-worker", 1500, 500, 10000);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}

void test_api_complete()
{
    esp_err_t err = api_complete(42, "test-worker", 2000, 1000, 20000);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}

void app_main(void)
{
    ESP_LOGI(TAG, "Starting API Client tests...");
    // Initial hardware init
    nvs_flash_init();
    wifi_init_sta();

    UNITY_BEGIN();
    RUN_TEST(test_api_lease_success);
    RUN_TEST(test_api_checkpoint);
    RUN_TEST(test_api_complete);
    UNITY_END();
}
