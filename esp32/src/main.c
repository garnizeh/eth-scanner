#include <stdio.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_system.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "wifi_handler.h"
#include "global_state.h"
#include "nvs_handler.h"
#include "nvs_compat.h"
#include "benchmark.h"

static const char *TAG = "eth-scanner";

// Temporary stub for batch size calculation (to be implemented in P07-T090)
uint32_t calculate_batch_size(uint32_t keys_per_sec, uint32_t target_duration_sec)
{
    return keys_per_sec * target_duration_sec;
}

// Extracted helper so tests can exercise retry logic.
esp_err_t nvs_init_with_retry(void)
{
    // Use wrapper functions so unit tests can override behavior.
    esp_err_t ret = nvs_flash_init_wr();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND)
    {
        // NVS partition was truncated, erase and retry
        esp_err_t erase_ret = nvs_flash_erase_wr();
        if (erase_ret != ESP_OK)
        {
            return erase_ret;
        }
        ret = nvs_flash_init_wr();
    }
    return ret;
}

// Declare app_main as weak so it can be overridden by test code
void app_main(void) __attribute__((weak));
void app_main(void)
{
    // Initialize NVS
    esp_err_t ret = nvs_init_with_retry();
    ESP_ERROR_CHECK(ret);

    // Initialize and open storage namespace in NVS
    esp_err_t err = nvs_handler_init();
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "NVS handler init failed: %s", esp_err_to_name(err));
        return;
    }

    ESP_LOGI(TAG, "EthScanner ESP32 Worker starting...");

    // Run startup benchmark for throughput calculation
    ESP_LOGI(TAG, "Running startup benchmark...");
    uint32_t throughput = benchmark_key_generation();
    g_state.keys_per_second = throughput;
    ESP_LOGI(TAG, "Device throughput: %lu keys/sec", (unsigned long)throughput);

    // Initial batch size calculation (to be refined in P07-T090)
    uint32_t batch_size = calculate_batch_size(throughput, 3600);
    ESP_LOGI(TAG, "Initial calculated batch size: %lu", (unsigned long)batch_size);

    // Initialize WiFi
    wifi_init_sta();

    ESP_LOGI(TAG, "WiFi connected, starting worker tasks...");

    while (1)
    {
        vTaskDelay(pdMS_TO_TICKS(1000));
    }
}
