#include <unity.h>
#include <string.h>
#include "esp_timer.h"
#include "nvs_handler.h"
#include "shared_types.h"
#include "nvs_compat.h"

extern int stub_nvs_set_blob_error;
extern int stub_nvs_commit_error;
extern int nvs_commit_count;

void test_save_checkpoint_success(void)
{
    nvs_commit_count = 0;
    stub_nvs_set_blob_error = 0;
    stub_nvs_commit_error = 0;

    job_checkpoint_t checkpoint = {
        .job_id = 12345,
        .nonce_start = 1000,
        .nonce_end = 2000,
        .current_nonce = 1500,
        .keys_scanned = 500};
    memset(checkpoint.prefix_28, 0xAA, PREFIX_28_SIZE);

    esp_err_t err = save_checkpoint((nvs_handle_t)0x1234, &checkpoint);
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(1, nvs_commit_count);

    // Verify written data
    job_checkpoint_t read_ckpt;
    size_t length = sizeof(job_checkpoint_t);
    err = nvs_get_blob_wr((nvs_handle_t)0x1234, "job_ckpt", &read_ckpt, &length);
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(12345, read_ckpt.job_id);
    TEST_ASSERT_EQUAL(1500, read_ckpt.current_nonce);
    TEST_ASSERT_EQUAL(500, read_ckpt.keys_scanned);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(checkpoint.prefix_28, read_ckpt.prefix_28, PREFIX_28_SIZE);
    TEST_ASSERT_EQUAL(0xDEADBEEF, read_ckpt.magic);
}

void test_save_checkpoint_null_arg(void)
{
    esp_err_t err = save_checkpoint((nvs_handle_t)0x1234, NULL);
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_ARG, err);
}

void test_save_checkpoint_set_blob_error(void)
{
    stub_nvs_set_blob_error = 1;
    job_checkpoint_t checkpoint = {.job_id = 1};

    esp_err_t err = save_checkpoint((nvs_handle_t)0x1234, &checkpoint);
    TEST_ASSERT_NOT_EQUAL(ESP_OK, err);

    stub_nvs_set_blob_error = 0;
}

void test_save_checkpoint_commit_error(void)
{
    stub_nvs_commit_error = 1;
    job_checkpoint_t checkpoint = {.job_id = 1};

    esp_err_t err = save_checkpoint((nvs_handle_t)0x1234, &checkpoint);
    TEST_ASSERT_NOT_EQUAL(ESP_OK, err);

    stub_nvs_commit_error = 0;
}

void test_load_checkpoint_success(void)
{
    // First save a valid checkpoint
    job_checkpoint_t save_ckpt = {
        .job_id = 98765,
        .current_nonce = 42000,
        .timestamp = esp_timer_get_time() / 1000000ULL};
    save_checkpoint((nvs_handle_t)0x1234, &save_ckpt);

    job_checkpoint_t read_ckpt;
    esp_err_t err = load_checkpoint((nvs_handle_t)0x1234, &read_ckpt);
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(98765, read_ckpt.job_id);
    TEST_ASSERT_EQUAL(42000, read_ckpt.current_nonce);
    TEST_ASSERT_EQUAL(0xDEADBEEF, read_ckpt.magic);
}

void test_load_checkpoint_not_found(void)
{
    // Clear blob length in stub to simulate NOT_FOUND
    extern size_t g_test_nvs_blob_len;
    g_test_nvs_blob_len = 0;

    job_checkpoint_t read_ckpt;
    esp_err_t err = load_checkpoint((nvs_handle_t)0x1234, &read_ckpt);
    TEST_ASSERT_EQUAL(ESP_ERR_NOT_FOUND, err);
}

void test_load_checkpoint_invalid_magic(void)
{
    // Save valid checkpoint then corrupt magic
    job_checkpoint_t ckpt = {.job_id = 1, .timestamp = esp_timer_get_time() / 1000000ULL};
    save_checkpoint((nvs_handle_t)0x1234, &ckpt);

    // Manual corruption via NVS wrapper access
    job_checkpoint_t corrupt;
    size_t len = sizeof(job_checkpoint_t);
    nvs_get_blob_wr((nvs_handle_t)0x1234, "job_ckpt", &corrupt, &len);
    corrupt.magic = 0xBAD0FEED;
    nvs_set_blob_wr((nvs_handle_t)0x1234, "job_ckpt", &corrupt, sizeof(job_checkpoint_t));

    esp_err_t err = load_checkpoint((nvs_handle_t)0x1234, &ckpt);
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_CRC, err);
}

void test_load_checkpoint_stale(void)
{
    // Simulate old checkpoint or one from "future" (which handles boot time wraps)
    job_checkpoint_t ckpt = {.job_id = 1, .timestamp = esp_timer_get_time() / 1000000ULL};
    save_checkpoint((nvs_handle_t)0x1234, &ckpt);

    // Corrupt timestamp to be too far in the past OR in the future relative to low 'now'
    job_checkpoint_t stale;
    size_t len = sizeof(job_checkpoint_t);
    nvs_get_blob_wr((nvs_handle_t)0x1234, "job_ckpt", &stale, &len);

    // Using a very large value to ensure it's "future" relative to low 'now' and triggers staleness check
    stale.timestamp = 0x7FFFFFFFFFFFFFFFULL;
    nvs_set_blob_wr((nvs_handle_t)0x1234, "job_ckpt", &stale, sizeof(job_checkpoint_t));

    esp_err_t err = load_checkpoint((nvs_handle_t)0x1234, &ckpt);
    TEST_ASSERT_EQUAL(ESP_ERR_INVALID_STATE, err);
}
