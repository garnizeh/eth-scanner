#include <unity.h>
#include "api_client.h"
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"
#include <string.h>

void test_api_lease_success()
{
    job_info_t job;
    esp_err_t err = api_lease_job("test-worker", 5000, &job);
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(42, job.job_id);
    TEST_ASSERT_EQUAL(1000, job.nonce_start);
    TEST_ASSERT_EQUAL(2000, job.nonce_end);

    // Target address: 0x742d3... = [0x74, 0x2d, 0x35, 0xcc, ...]
    uint8_t expected_target[20] = {
        0x74, 0x2d, 0x35, 0xCc, 0x66, 0x34, 0xC0, 0x53, 0x29, 0x25,
        0xa3, 0xb8, 0x44, 0xBc, 0x45, 0x4e, 0x44, 0x38, 0xf4, 0x4e};
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected_target, job.target_address, 20);

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

void test_api_submit_result()
{
    uint8_t priv_key[32] = {0x01, 0x02, 0x03, 0x04};
    uint8_t address[20] = {0xaa, 0xbb, 0xcc, 0xdd};

    esp_err_t err = api_submit_result(42, "test-worker", priv_key, address);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}
