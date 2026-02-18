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
#include "eth_crypto.h"
#include <string.h>
#include <time.h>

// Define global state instance and initialize
global_state_t g_state = {0};

static const char *TAG = "eth-scanner";

// Job configuration
#define TARGET_DURATION_SEC 3600 // 1 hour

// Task prototypes
void core0_system_task(void *pvParameters);
void core1_worker_task(void *pvParameters);

/**
 * @brief Optimally updates the 4-byte nonce at the end of a 32-byte private key.
 *
 * This function performs direct byte manipulation to avoid expensive sprintf/memcpy.
 * The nonce is placed at offset 28 in little-endian format.
 *
 * @param buffer 32-byte private key buffer.
 * @param nonce  4-byte nonce to set.
 */
static inline void update_nonce_in_buffer(uint8_t *buffer, uint32_t nonce)
{
    buffer[28] = (uint8_t)(nonce & 0xFF);
    buffer[29] = (uint8_t)((nonce >> 8) & 0xFF);
    buffer[30] = (uint8_t)((nonce >> 16) & 0xFF);
    buffer[31] = (uint8_t)((nonce >> 24) & 0xFF);
}

// Static task buffers for Core 0 (System management)
#define CORE0_STACK_SIZE 4096
static StackType_t core0_stack[CORE0_STACK_SIZE];
static StaticTask_t core0_task_buffer;

// Static task buffers for Core 1 (computational hot loop)
#define CORE1_STACK_SIZE 8192
static StackType_t core1_stack[CORE1_STACK_SIZE];
static StaticTask_t core1_task_buffer;

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

        // Handle Result Found Signal
        if (notifications & NOTIFY_BIT_RESULT_FOUND)
        {
            ESP_LOGI(TAG, "!!! MATCH FOUND Signal received from Core 1 !!!");

            found_result_t res;
            while (xQueueReceive(g_state.found_results_queue, &res, 0) == pdTRUE)
            {
                ESP_LOGI(TAG, "Processing result from queue for job %lld", res.job_id);

                if (g_state.wifi_connected)
                {
                    uint8_t derived_addr[20];
                    derive_eth_address(res.private_key, derived_addr);
                    api_submit_result(res.job_id, g_state.worker_id, res.private_key, derived_addr);
                }
                else
                {
                    ESP_LOGW(TAG, "Match found for job %lld but WiFi disconnected. Result dropped (not persisted in MVP).", res.job_id);
                }
            }

            // For now, let's keep the existing logic that one match stops the CURRENT job session
            // if you found YOUR target. But wait, if it continues scanning, it just reports more.
            // Let's keep job_active as it is for now (not setting it false here immediately)
            // OR we can decide to stop.
            // In the SDD, we usually lease one range per worker.
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
void core1_worker_task(void *pvParameters)
{
    ESP_LOGI(TAG, "Starting Computation Task on Core %d with priority %d",
             xPortGetCoreID(), uxTaskPriorityGet(NULL));

    uint32_t notifications = 0;
    uint8_t priv_key[32] __attribute__((aligned(4))) = {0};

    while (1)
    {
        // Wait for notification from Core 0
        if (xTaskNotifyWait(0, 0xFFFFFFFF, &notifications, pdMS_TO_TICKS(100)) == pdTRUE)
        {
            if (notifications & NOTIFY_BIT_JOB_LEASED)
            {
                ESP_LOGI(TAG, "Core 1: New job signaled! Starting scan for job %lld...", g_state.current_job.job_id);

                // Initialize job-specific state
                memcpy(priv_key, g_state.current_job.prefix_28, 28);
                uint32_t start = (uint32_t)g_state.current_job.nonce_start;
                uint32_t end = (uint32_t)g_state.current_job.nonce_end;
                uint32_t current = start;

                // Reset atomic progress trackers for the new job session
                atomic_store(&g_state.current_nonce, start);
                atomic_store(&g_state.keys_scanned, 0);

                ESP_LOGI(TAG, "Core 1: Scan loop starting: %lu -> %lu", (unsigned long)start, (unsigned long)end);

                while (g_state.job_active && !g_state.should_stop)
                {
                    if (current > end)
                    {
                        break;
                    }

                    // Optimized byte-level nonce manipulation (P08-T080)
                    update_nonce_in_buffer(priv_key, current);

                    // Derive Ethereum address from the current private key (P08-T100)
                    uint8_t derived_addr[20];
                    derive_eth_address(priv_key, derived_addr);

                    // Binary comparison using memcmp for zero-overhead validation (P08-T090)
                    if (memcmp(derived_addr, g_state.current_job.target_address, 20) == 0)
                    {
                        ESP_LOGI(TAG, "Core 1: MATCH FOUND at nonce %lu", (unsigned long)current);

                        found_result_t res;
                        res.job_id = g_state.current_job.job_id;
                        memcpy(res.private_key, priv_key, 32);

                        if (xQueueSend(g_state.found_results_queue, &res, 0) == pdTRUE)
                        {
                            xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_RESULT_FOUND, eSetBits);
                        }
                        else
                        {
                            ESP_LOGE(TAG, "Core 1: FAILED TO QUEUE RESULT! Queue full.");
                        }

                        // According to P08-T100, Core 1 can continue scanning.
                        // However, for this MVP we usually stop after one match per range.
                        // We will just break here to keep it simple, or we can keep it running.
                        // The requirement says "Core 1 can continue scanning", so let's keep it running.
                        // break;
                    }

                    // Increment and update global progress
                    current++;
                    atomic_fetch_add(&g_state.current_nonce, 1);
                    atomic_fetch_add(&g_state.keys_scanned, 1);

                    // Periodically yield to avoid triggering task watchdogs on other cores
                    // or starving high-level maintenance tasks on this core.
                    // Every 256 keys (~0.7s at peak throughput) is a balanced yielding period.
                    if ((current & 0xFF) == 0)
                    {
                        vTaskDelay(pdMS_TO_TICKS(1));
                    }
                }

                if (current > end)
                {
                    ESP_LOGI(TAG, "Core 1: Job range completed successfully.");
                    xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_JOB_COMPLETE, eSetBits);
                }
            }
        }

        // Feed the watchdog by yielding when idle
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

    // Create result submission queue
    g_state.found_results_queue = xQueueCreate(5, sizeof(found_result_t));
    if (g_state.found_results_queue == NULL)
    {
        ESP_LOGE(TAG, "Failed to create result queue!");
        return;
    }

    ESP_LOGI(TAG, "Global state initialized (Queue created) for worker: %s", g_state.worker_id);

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

    g_state.core0_task_handle = xTaskCreateStaticPinnedToCore(
        core0_system_task,
        "core0_system",
        CORE0_STACK_SIZE,
        NULL,
        5, // Priority must be lower than Core 1 to avoid interference
        core0_stack,
        &core0_task_buffer,
        0 // Core 0
    );

    if (g_state.core0_task_handle == NULL)
    {
        ESP_LOGE(TAG, "Failed to create Core 0 system task!");
    }

    // Create Core 1 task statically for maximum stability and priority
    g_state.core1_task_handle = xTaskCreateStaticPinnedToCore(
        core1_worker_task,
        "core1_worker",
        CORE1_STACK_SIZE,
        NULL,
        configMAX_PRIORITIES - 1, // Highest priority for the computational hot-loop
        core1_stack,
        &core1_task_buffer,
        1 // APP_CPU (Core 1)
    );

    if (g_state.core1_task_handle == NULL)
    {
        ESP_LOGE(TAG, "Failed to create Core 1 worker task!");
    }

    ESP_LOGI(TAG, "All tasks spawned.");

    // Monitoring loop for the main task
    while (1)
    {
        vTaskDelay(pdMS_TO_TICKS(10000));
    }
}
