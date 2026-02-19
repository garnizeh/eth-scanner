#include <stdio.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_system.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "wifi_handler.h"
#include "shared_types.h"
#include "nvs_handler.h"
#include "benchmark.h"
#include "batch_calculator.h"
#include "eth_crypto.h"
#include "led_manager.h"
#include "core_tasks.h"
#include "config.h"
#include <string.h>

// Define global state instance and initialize
global_state_t g_state = {0};

const char *TAG = "eth-scanner";

// Declare app_main as weak so it can be overridden by test code
void app_main(void) __attribute__((weak));
void app_main(void)
{
    // Initialize LED Manager for status feedback
    led_manager_init();
    set_led_status(LED_SYSTEM_ERROR); // Default until system is ready

    // Initialize global state
    memset(&g_state, 0, sizeof(global_state_t));

    // Set worker ID from config
#ifdef CONFIG_ETHSCANNER_WORKER_ID
    strncpy(g_state.worker_id, CONFIG_ETHSCANNER_WORKER_ID, WORKER_ID_MAX_LEN - 1);
#else
    strncpy(g_state.worker_id, "esp32-default", WORKER_ID_MAX_LEN - 1);
#endif

    // Initialize atomic counters
    atomic_init(&g_state.current_nonce, 0);
    atomic_init(&g_state.keys_scanned, 0);

    // Create result submission queue
    g_state.found_results_queue = xQueueCreate(5, sizeof(found_result_t));
    if (g_state.found_results_queue == NULL)
    {
        ESP_LOGE(TAG, "Failed to create result queue!");
        set_led_status(LED_SYSTEM_ERROR);
        return;
    }

    ESP_LOGI(TAG, "Global state initialized for worker: %s", g_state.worker_id);

    // Initialize NVS Flash
    if (nvs_init_with_retry() != ESP_OK)
    {
        ESP_LOGE(TAG, "NVS flash recovery failed!");
        set_led_status(LED_SYSTEM_ERROR);
        return;
    }

    // Initialize and open storage namespace in NVS
    if (nvs_handler_init() != ESP_OK)
    {
        ESP_LOGE(TAG, "NVS handler init failed!");
        set_led_status(LED_SYSTEM_ERROR);
        return;
    }

    ESP_LOGI(TAG, "EthScanner ESP32 Worker starting...");

    // P08-T120: Check for existing checkpoint in NVS before starting
    job_resume_from_nvs();

    // Run startup benchmark for throughput calculation
    uint32_t throughput = benchmark_key_generation();
    g_state.stats.keys_per_second = throughput;
    ESP_LOGI(TAG, "Device throughput: %lu keys/sec", (unsigned long)throughput);

    // Initial batch size calculation based on TARGET_DURATION_SEC (3600s)
    uint32_t batch_size = calculate_batch_size(throughput, TARGET_DURATION_SEC);
    ESP_LOGI(TAG, "Initial batch size: %lu keys", (unsigned long)batch_size);

    // Create and start core tasks (Core 0/1) and the checkpoint timer
    start_core_tasks();

    ESP_LOGI(TAG, "System operational.");

    // Monitoring loop for the main task
    while (1)
    {
        vTaskDelay(pdMS_TO_TICKS(10000));
    }
}
