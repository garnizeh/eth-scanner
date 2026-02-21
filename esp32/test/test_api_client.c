#include <unity.h>
#include "api_client.h"
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"
#include <string.h>

extern void set_mock_http_response(int status, const char *json_body);

void test_api_lease_success()
{
    // Mock response for leasing a job
    const char *mock_response =
        "{"
        "\"job_id\": 42,"
        "\"nonce_start\": 1000,"
        "\"nonce_end\": 2000,"
        "\"prefix_28\": \"AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHA==\","
        "\"target_addresses\": [\"742d35Cc6634C0532925a3b844Bc454e4438f44e\"]"
        "}";
    set_mock_http_response(200, mock_response);

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
    TEST_ASSERT_EQUAL(1, job.num_targets);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected_target, job.target_addresses[0], 20);

    // Prefix 1..28
    uint8_t expected_prefix[28];
    for (int i = 0; i < 28; i++)
        expected_prefix[i] = i + 1;
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected_prefix, job.prefix_28, 28);
}

void test_api_checkpoint()
{
    set_mock_http_response(200, NULL);
    esp_err_t err = api_checkpoint(42, "test-worker", 1500, 500, 10000);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}

void test_api_complete()
{
    set_mock_http_response(200, NULL);
    esp_err_t err = api_complete(42, "test-worker", 2000, 1000, 20000);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}

void test_api_submit_result()
{
    set_mock_http_response(200, NULL);
    uint8_t priv_key[32];
    uint8_t address[20];
    memset(priv_key, 0x01, sizeof(priv_key));
    memset(address, 0xAA, sizeof(address));

    esp_err_t err = api_submit_result(42, "test-worker", priv_key, address, 123456);
    TEST_ASSERT_EQUAL(ESP_OK, err);
}

// Resiliency Tests for Job Synchronization

void test_checkpoint_404_rejected()
{
    // Simulate server returning 404 for a checkpoint
    set_mock_http_response(404, NULL);

    esp_err_t err = api_checkpoint(999, "test-worker", 500, 500, 1000);
    // Should return ESP_ERR_INVALID_STATE based on our recent changes
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_STATE, err);
}

void test_checkpoint_410_rejected()
{
    // Simulate server returning 410 (Gone) for a checkpoint
    set_mock_http_response(410, NULL);

    esp_err_t err = api_checkpoint(999, "test-worker", 500, 500, 1000);
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_STATE, err);
}

void test_complete_410_rejected()
{
    set_mock_http_response(410, NULL);

    esp_err_t err = api_complete(999, "test-worker", 10000, 10000, 5000);
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_STATE, err);
}

extern global_state_t g_state;

void test_result_queue_flow()
{
    // Initialize queue if not done
    if (g_state.found_results_queue == NULL)
    {
        g_state.found_results_queue = xQueueCreate(5, sizeof(found_result_t));
    }
    TEST_ASSERT_NOT_NULL(g_state.found_results_queue);

    // Clear queue
    found_result_t dummy;
    while (xQueueReceive(g_state.found_results_queue, &dummy, 0) == pdTRUE)
        ;

    // Push test result
    found_result_t res;
    res.job_id = 1234;
    res.nonce_found = 5678;
    memset(res.private_key, 0xDD, 32);

    BaseType_t q_ret = xQueueSend(g_state.found_results_queue, &res, 0);
    TEST_ASSERT_EQUAL(pdTRUE, q_ret);

    // Simulate what Core 0 does: read from queue and verify
    found_result_t read_res;
    q_ret = xQueueReceive(g_state.found_results_queue, &read_res, 0);
    TEST_ASSERT_EQUAL(pdTRUE, q_ret);
    TEST_ASSERT_EQUAL(1234, read_res.job_id);
    TEST_ASSERT_EQUAL(5678, read_res.nonce_found);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(res.private_key, read_res.private_key, 32);

    // Since we are in simulation, we don't call api_submit_result here to avoid
    // actually sending matching-result packets in a generic unit test
    // (it would fail without IP or mock server).
}
