#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "unity.h"
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"

// External test function declarations
extern void test_api_lease_success(void);
extern void test_api_checkpoint(void);
extern void test_api_complete(void);

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
extern void test_load_checkpoint_stale(void);

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

    RUN_TEST(test_save_checkpoint_success);
    RUN_TEST(test_save_checkpoint_null_arg);
    RUN_TEST(test_save_checkpoint_set_blob_error);
    RUN_TEST(test_save_checkpoint_commit_error);
    RUN_TEST(test_load_checkpoint_success);
    RUN_TEST(test_load_checkpoint_not_found);
    RUN_TEST(test_load_checkpoint_invalid_magic);
    RUN_TEST(test_load_checkpoint_stale);

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
    }
    else
    {
        ESP_LOGE(TAG, "WiFi connection timed out. Skipping API tests.");
        // Unity doesn't have a "Skip" macro for entire groups,
        // but the logs will clearly show why these didn't run.
    }

    UNITY_END();

    /* * Finish the task. vTaskDelete ensures the memory used by this task
     * is reclaimed by the Idle Task.
     */
    ESP_LOGI(TAG, "Testing complete.");
    vTaskDelete(NULL);
}