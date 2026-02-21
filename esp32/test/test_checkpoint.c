#include <unity.h>
#include <string.h>
#include <stdatomic.h>
#include "esp_timer.h"
#include "nvs_handler.h"
#include "shared_types.h"
#include "nvs_compat.h"

extern int stub_nvs_set_blob_error;
extern int stub_nvs_commit_error;
extern int nvs_commit_count;
extern global_state_t g_state;

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
        .keys_scanned = 500,
        .magic = 0xDEADBEEF};
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
    job_checkpoint_t checkpoint = {.job_id = 1, .magic = 0xDEADBEEF};

    esp_err_t err = save_checkpoint((nvs_handle_t)0x1234, &checkpoint);
    TEST_ASSERT_NOT_EQUAL(ESP_OK, err);

    stub_nvs_set_blob_error = 0;
}

void test_save_checkpoint_commit_error(void)
{
    stub_nvs_commit_error = 1;
    job_checkpoint_t checkpoint = {.job_id = 1, .magic = 0xDEADBEEF};

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
        .timestamp = esp_timer_get_time() / 1000000ULL,
        .magic = 0xDEADBEEF};
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
    job_checkpoint_t ckpt = {.job_id = 1, .timestamp = esp_timer_get_time() / 1000000ULL, .magic = 0xDEADBEEF};
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

void test_recovery_logic_resumption(void)
{
    // 1. Prepare global state with a mock job
    memset(&g_state, 0, sizeof(global_state_t));
    g_state.current_job.job_id = 12345;
    memset(g_state.current_job.prefix_28, 0x12, PREFIX_28_SIZE);
    g_state.current_job.nonce_start = 1000;
    g_state.current_job.nonce_end = 2000;
    atomic_init(&g_state.current_nonce, 1500);
    atomic_init(&g_state.keys_scanned, 500);

    // 2. Save checkpoint
    job_checkpoint_t save_ckpt = {
        .job_id = 12345,
        .nonce_start = 1000,
        .nonce_end = 2000,
        .current_nonce = 1500,
        .keys_scanned = 500,
        .timestamp = 99999,
        .magic = 0xDEADBEEF};
    memcpy(save_ckpt.prefix_28, g_state.current_job.prefix_28, PREFIX_28_SIZE);
    save_checkpoint((nvs_handle_t)0x1234, &save_ckpt);

    // 3. Clear global state to simulate "reset"
    memset(&g_state, 0, sizeof(global_state_t));

    // 4. Mimic recovery logic from app_main
    job_checkpoint_t recovered_ckpt;
    esp_err_t err = load_checkpoint((nvs_handle_t)0x1234, &recovered_ckpt);
    TEST_ASSERT_EQUAL(ESP_OK, err);

    // Apply recovered state to g_state
    g_state.current_job.job_id = recovered_ckpt.job_id;
    memcpy(g_state.current_job.prefix_28, recovered_ckpt.prefix_28, PREFIX_28_SIZE);
    g_state.current_job.nonce_start = recovered_ckpt.nonce_start;
    g_state.current_job.nonce_end = recovered_ckpt.nonce_end;
    atomic_init(&g_state.current_nonce, recovered_ckpt.current_nonce);
    atomic_init(&g_state.keys_scanned, recovered_ckpt.keys_scanned);

    // 5. Verify g_state
    TEST_ASSERT_EQUAL(12345, g_state.current_job.job_id);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(save_ckpt.prefix_28, g_state.current_job.prefix_28, PREFIX_28_SIZE);
    TEST_ASSERT_EQUAL(1500, (uint32_t)atomic_load(&g_state.current_nonce));
    TEST_ASSERT_EQUAL(500, (uint32_t)atomic_load(&g_state.keys_scanned));
}
