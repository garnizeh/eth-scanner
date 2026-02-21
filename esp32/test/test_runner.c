#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "unity.h"
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"
#include "led_manager.h"

// External test function declarations
extern void test_api_lease_success(void);
extern void test_api_checkpoint(void);
extern void test_api_complete(void);
extern void test_api_submit_result(void);
extern void test_checkpoint_404_rejected(void);
extern void test_checkpoint_410_rejected(void);
extern void test_complete_410_rejected(void);
extern void test_result_queue_flow(void);

extern void test_crypto_secp256k1_point_multiplication(void);
extern void test_crypto_keccak256(void);
extern void test_crypto_derive_eth_address(void);
extern void test_crypto_address_comparison(void);

extern void test_nvs_handler_success(void);
extern void test_nvs_handler_open_error(void);
extern void test_nvs_handler_stats_warning(void);
extern void test_nvs_init_erase_retry(void);

extern void test_save_checkpoint_success(void);
extern void test_save_checkpoint_null_arg(void);
extern void test_save_checkpoint_set_blob_error(void);
extern void test_save_checkpoint_commit_error(void);

extern void test_load_checkpoint_success(void);
extern void test_load_checkpoint_not_found(void);
extern void test_load_checkpoint_invalid_magic(void);
extern void test_clear_checkpoint_manual(void);
extern void test_job_resume_clears_if_invalid_magic(void);
extern void test_recovery_logic_resumption(void);

extern void test_benchmark_positive_throughput(void);
extern void test_benchmark_repeatability(void);

extern void test_batch_calc_normal(void);
extern void test_batch_calc_small_throughput(void);
extern void test_batch_calc_zero_throughput(void);
extern void test_batch_calc_zero_duration(void);
extern void test_batch_calc_mid_range(void);

extern void test_led_manager_init(void);
extern void test_led_set_status(void);
extern void test_led_trigger_activity(void);

static const char *TAG = "test_runner";

void app_main(void)
{
    // Give the serial monitor and the hardware a moment to stabilize
    vTaskDelay(pdMS_TO_TICKS(1000));
    ESP_LOGI(TAG, "Starting test runner...");

    /* * STAGE 1: NVS INITIALIZATION
     * Initialize NVS early as it's required for both NVS tests and WiFi calibration.
     */
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND)
    {
        ESP_LOGI(TAG, "NVS partition issue detected, erasing and reinitializing...");

        ESP_ERROR_CHECK(nvs_flash_erase());
        ESP_ERROR_CHECK(nvs_flash_init());
    }

    UNITY_BEGIN();

    /* * STAGE 2: UNIT TESTS (NO WIFI REQUIRED)
     * Run these first to ensure base logic is sound before radio interference.
     */
    ESP_LOGI(TAG, "Running NVS and Logic unit tests...");

    RUN_TEST(test_nvs_handler_success);
    RUN_TEST(test_nvs_handler_open_error);
    RUN_TEST(test_nvs_handler_stats_warning);
    RUN_TEST(test_nvs_init_erase_retry);

    ESP_LOGI(TAG, "Running Checkpoint tests...");

    RUN_TEST(test_save_checkpoint_success);
    RUN_TEST(test_save_checkpoint_null_arg);
    RUN_TEST(test_save_checkpoint_set_blob_error);
    RUN_TEST(test_save_checkpoint_commit_error);
    RUN_TEST(test_load_checkpoint_success);
    RUN_TEST(test_load_checkpoint_not_found);
    RUN_TEST(test_load_checkpoint_invalid_magic);
    RUN_TEST(test_clear_checkpoint_manual);
    RUN_TEST(test_job_resume_clears_if_invalid_magic);
    RUN_TEST(test_recovery_logic_resumption);
    RUN_TEST(test_benchmark_positive_throughput);
    RUN_TEST(test_benchmark_repeatability);

    ESP_LOGI(TAG, "Running Batch Calculator tests...");

    RUN_TEST(test_batch_calc_normal);
    RUN_TEST(test_batch_calc_small_throughput);
    RUN_TEST(test_batch_calc_zero_throughput);
    RUN_TEST(test_batch_calc_zero_duration);
    RUN_TEST(test_batch_calc_mid_range);

    ESP_LOGI(TAG, "Running Crypto tests...");
    RUN_TEST(test_crypto_secp256k1_point_multiplication);
    RUN_TEST(test_crypto_keccak256);
    RUN_TEST(test_crypto_derive_eth_address);
    RUN_TEST(test_crypto_address_comparison);

    ESP_LOGI(TAG, "Running LED Manager tests...");
    RUN_TEST(test_led_manager_init);
    RUN_TEST(test_led_set_status);
    RUN_TEST(test_led_trigger_activity);

    /* * STAGE 3: WIFI INITIALIZATION
     * Only start WiFi after local tests are done to avoid shared resource conflicts.
     * Ensure your wifi_init_sta() no longer uses portMAX_DELAY.
     */
    ESP_LOGI(TAG, "Initializing WiFi for integration tests...");
    wifi_init_sta();

    int retry = 0;
    const int max_retries = 20; // 10 seconds timeout (20 * 500ms)

    while (retry < max_retries && !is_wifi_connected())
    {
        vTaskDelay(pdMS_TO_TICKS(500));
        if (retry % 4 == 0)
        {
            ESP_LOGI(TAG, "Waiting for WiFi connection...");
        }
        retry++;
    }

    /* * STAGE 4: INTEGRATION TESTS (WIFI REQUIRED)
     * Only run these if the radio successfully acquired an IP.
     */
    if (is_wifi_connected())
    {
        ESP_LOGI(TAG, "WiFi Connected! Starting API tests...");

        RUN_TEST(test_api_lease_success);
        RUN_TEST(test_api_checkpoint);
        RUN_TEST(test_api_complete);
        RUN_TEST(test_api_submit_result);
        RUN_TEST(test_checkpoint_404_rejected);
        RUN_TEST(test_checkpoint_410_rejected);
        RUN_TEST(test_complete_410_rejected);
        RUN_TEST(test_result_queue_flow);
    }
    else
    {
        ESP_LOGE(TAG, "WiFi connection timed out. Skipping API tests.");
        // Unity doesn't have a "Skip" macro for entire groups,
        // but the logs will clearly show why these didn't run.
    }

    set_led_status(LED_OFF);
    vTaskDelay(pdMS_TO_TICKS(150));

    UNITY_END();

    /* * Finish the task. vTaskDelete ensures the memory used by this task
     * is reclaimed by the Idle Task.
     */
    ESP_LOGI(TAG, "Testing complete.");
    vTaskDelete(NULL);
}