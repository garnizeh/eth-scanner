#include <stdio.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_system.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "wifi_handler.h"
#include "shared_types.h"
#include "nvs_handler.h"
#include "nvs_compat.h"
#include "benchmark.h"
#include "batch_calculator.h"
#include "api_client.h"
#include <string.h>
#include <time.h>

// Define global state instance and initialize
global_state_t g_state = {0};

static const char *TAG = "eth-scanner";

// Job configuration
#define TARGET_DURATION_SEC 3600 // 1 hour

// Task prototypes
void core0_system_task(void *pvParameters);
void core1_computation_task(void *pvParameters);

// Timer callback for periodic checkpoints
static void checkpoint_timer_callback(TimerHandle_t xTimer)
{
    if (g_state.job_active && g_state.core0_task_handle != NULL)
    {
        ESP_LOGI(TAG, "Checkpoint timer fired! Signaling Core 0...");
        xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_CHECKPOINT, eSetBits);
    }
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

// System Management Task (Networking, API, Monitoring) - Core 0
void core0_system_task(void *pvParameters)
{
    ESP_LOGI(TAG, "Starting System Task on Core %d", xPortGetCoreID());

    // Initialize WiFi
    wifi_init_sta();

    uint32_t notifications = 0;

    // Maintenance loop
    while (1)
    {
        // Wait for notifications or 1s timeout for basic maintenance
        notifications = 0;
        xTaskNotifyWait(0, 0xFFFFFFFF, &notifications, pdMS_TO_TICKS(1000));

        // Check WiFi status and update global state
        g_state.wifi_connected = is_wifi_connected();

        // Handle Checkpoint Signal
        if (notifications & NOTIFY_BIT_CHECKPOINT)
        {
            if (g_state.job_active)
            {
                uint64_t current = atomic_load(&g_state.current_nonce);
                uint64_t scanned = atomic_load(&g_state.keys_scanned);
                ESP_LOGI(TAG, "Periodic Checkpoint: [ID %lld] Nonce: %llu, Scanned: %llu",
                         g_state.current_job.job_id, (unsigned long long)current, (unsigned long long)scanned);

                // Save to NVS
                job_checkpoint_t cp = {0};
                cp.job_id = g_state.current_job.job_id;
                memcpy(cp.prefix_28, g_state.current_job.prefix_28, PREFIX_28_SIZE);
                cp.nonce_start = g_state.current_job.nonce_start;
                cp.nonce_end = g_state.current_job.nonce_end;
                cp.current_nonce = current;
                cp.keys_scanned = scanned;
                cp.timestamp = (uint64_t)time(NULL);
                cp.magic = 0xACE1; // Validating magic

                esp_err_t err = save_checkpoint(g_state.nvs_handle, &cp);
                if (err != ESP_OK)
                {
                    ESP_LOGE(TAG, "Failed to save checkpoint to NVS: %s", esp_err_to_name(err));
                }

                // If WiFi is connected, report to API as well
                if (g_state.wifi_connected)
                {
                    api_checkpoint(cp.job_id, g_state.worker_id, current, scanned, 0); // TODO: Track actual duration
                }
            }
        }

        // Handle Job Completion Signal
        if (notifications & NOTIFY_BIT_JOB_COMPLETE)
        {
            ESP_LOGI(TAG, "Job completion received from Core 1.");
            g_state.job_active = false;

            if (g_state.wifi_connected)
            {
                uint64_t current = atomic_load(&g_state.current_nonce);
                uint64_t scanned = atomic_load(&g_state.keys_scanned);
                api_complete(g_state.current_job.job_id, g_state.worker_id, current, scanned, 0);
            }
        }

        if (g_state.wifi_connected && !g_state.job_active)
        {
            ESP_LOGI(TAG, "Device idle, requesting new job lease...");

            // Calculate requested batch size based on startup benchmark
            uint32_t batch_size = calculate_batch_size(g_state.stats.keys_per_second, TARGET_DURATION_SEC);

            job_info_t new_job = {0};
            esp_err_t err = api_lease_job(g_state.worker_id, batch_size, &new_job);

            if (err == ESP_OK)
            {
                ESP_LOGI(TAG, "Job leased successfully! ID: %lld, Range: [%lu - %lu]",
                         new_job.job_id, (unsigned long)new_job.nonce_start, (unsigned long)new_job.nonce_end);

                // Update global state
                memcpy(&(g_state.current_job), &new_job, sizeof(job_info_t));
                atomic_store(&g_state.current_nonce, new_job.nonce_start);
                atomic_store(&g_state.keys_scanned, 0);
                g_state.job_active = true;

                // Signal Core 1 task to start working
                xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_JOB_LEASED, eSetBits);
            }
            else if (err == ESP_ERR_NOT_FOUND)
            {
                ESP_LOGW(TAG, "No jobs available on server, retrying soon...");
                vTaskDelay(pdMS_TO_TICKS(30000));
                continue;
            }
            else
            {
                ESP_LOGE(TAG, "Failed to lease job (err %d), retrying soon...", err);
                vTaskDelay(pdMS_TO_TICKS(10000));
                continue;
            }
        }

        // Feed watchdog by yielding
        vTaskDelay(pdMS_TO_TICKS(100));
    }
}

// Computation Task (The "Hot Loop") - Core 1
void core1_computation_task(void *pvParameters)
{
    ESP_LOGI(TAG, "Starting Computation Task on Core %d", xPortGetCoreID());

    uint32_t notifications = 0;

    while (1)
    {
        // Wait for notification from Core 0
        if (xTaskNotifyWait(0, 0xFFFFFFFF, &notifications, pdMS_TO_TICKS(100)) == pdTRUE)
        {
            if (notifications & NOTIFY_BIT_JOB_LEASED)
            {
                ESP_LOGI(TAG, "Core 1: New job signaled! Starting scan for job %lld...", g_state.current_job.job_id);

                // SCAN LOOP PLACEHOLDER (To be implemented in P08-T060)
                // For now, we simulate work by incrementing counters and waiting
                while (g_state.job_active && !g_state.should_stop)
                {
                    uint64_t current = atomic_load(&g_state.current_nonce);
                    if (current >= g_state.current_job.nonce_end)
                    {
                        break;
                    }

                    // Simulate scanning 1000 keys
                    atomic_fetch_add(&g_state.current_nonce, 1000);
                    atomic_fetch_add(&g_state.keys_scanned, 1000);

                    // Yield to system tasks
                    vTaskDelay(pdMS_TO_TICKS(10));
                }

                if (atomic_load(&g_state.current_nonce) >= g_state.current_job.nonce_end)
                {
                    ESP_LOGI(TAG, "Core 1: Job complete.");
                    xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_JOB_COMPLETE, eSetBits);
                }
            }
        }

        // Feed the watchdog by yielding
        vTaskDelay(pdMS_TO_TICKS(100));
    }
}

// Declare app_main as weak so it can be overridden by test code
void app_main(void) __attribute__((weak));
void app_main(void)
{
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

    ESP_LOGI(TAG, "Global state initialized for worker: %s", g_state.worker_id);

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
    g_state.stats.keys_per_second = throughput;
    ESP_LOGI(TAG, "Device throughput: %lu keys/sec", (unsigned long)throughput);

    // Initial batch size calculation based on TARGET_DURATION_SEC (3600s)
    uint32_t batch_size = calculate_batch_size(throughput, TARGET_DURATION_SEC);
    ESP_LOGI(TAG, "Initial calculated batch size: %lu keys", (unsigned long)batch_size);

    // Create checkpoint timer
    g_state.checkpoint_timer = xTimerCreate("checkpoint_timer",
                                            pdMS_TO_TICKS(CHECKPOINT_INTERVAL_MS),
                                            pdTRUE, // Auto-reload
                                            (void *)0,
                                            checkpoint_timer_callback);

    if (g_state.checkpoint_timer != NULL)
    {
        xTimerStart(g_state.checkpoint_timer, 0);
        ESP_LOGI(TAG, "Checkpoint timer initialized (interval: %d ms)", CHECKPOINT_INTERVAL_MS);
    }

    // Create tasks pinned to cores
    // Core 0: PRO_CPU (Networking, API, Misc)
    // Core 1: APP_CPU (Hot Loop)

    xTaskCreatePinnedToCore(
        core0_system_task,
        "core0_system",
        4096,
        NULL,
        5, // Priority must be lower than Core 1 to avoid interference?
        &g_state.core0_task_handle,
        0 // Core 0
    );

    xTaskCreatePinnedToCore(
        core1_computation_task,
        "core1_compute",
        8192,
        NULL,
        10, // Higher priority for the compute task
        &g_state.core1_task_handle,
        1 // Core 1
    );

    ESP_LOGI(TAG, "All tasks spawned.");

    // Monitoring loop for the main task
    while (1)
    {
        vTaskDelay(pdMS_TO_TICKS(10000));
    }
}
