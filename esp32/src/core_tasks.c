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
#include "esp_timer.h"
#include <string.h>
#include <time.h>
#include "core_tasks.h"
#include "config.h"
#include "led_manager.h"

/* Static task buffers for Core 0 (System management) */
#define CORE0_STACK_SIZE 12288 // Increased from 4096 to accommodate JSON/HTTP stack usage
static StackType_t core0_stack[CORE0_STACK_SIZE];
static StaticTask_t core0_task_buffer;

/* Static task buffers for Core 1 (computational hot loop) */
#define CORE1_STACK_SIZE 8192
static StackType_t core1_stack[CORE1_STACK_SIZE];
static StaticTask_t core1_task_buffer;

static bool start_core1_task(void)
{
    if (g_state.core1_task_handle != NULL)
    {
        return true;
    }

    g_state.core1_task_handle = xTaskCreateStaticPinnedToCore(
        core1_worker_task,
        "core1_worker",
        CORE1_STACK_SIZE,
        NULL,
        10,
        core1_stack,
        &core1_task_buffer,
        1 // APP_CPU (Core 1)
    );

    if (g_state.core1_task_handle == NULL)
    {
        ESP_LOGE(TAG, "Failed to create Core 1 worker task!");
        return false;
    }

    ESP_LOGI(TAG, "Core 1 worker task created.");
    return true;
}

static void stop_core1_task(void)
{
    if (g_state.core1_task_handle == NULL)
    {
        return;
    }

    g_state.job_active = false;
    vTaskDelete(g_state.core1_task_handle);
    g_state.core1_task_handle = NULL;
    set_led_status(LED_WIFI_CONNECTING);
    ESP_LOGW(TAG, "Core 1 worker task stopped.");
}

/**
 * @brief Spawns Core 0 task. Core 1 will start after WiFi connects.
 */
void start_core_tasks(void)
{
    // Create tasks pinned to cores — Core 0 handles system and interrupts
    // Core 0: PRO_CPU (Networking, API, Misc)
    g_state.core0_task_handle = xTaskCreateStaticPinnedToCore(
        core0_system_task,
        "core0_system",
        CORE0_STACK_SIZE,
        NULL,
        8, // Increased priority (above default event loop task)
        core0_stack,
        &core0_task_buffer,
        0 // Core 0
    );

    if (g_state.core0_task_handle == NULL)
    {
        ESP_LOGE(TAG, "Failed to create Core 0 system task!");
    }

    ESP_LOGI(TAG, "Core 0 system task spawned. Core 1 will start only after WiFi connects.");
}

// System Management Task (Networking, API, Monitoring) - Core 0
void core0_system_task(void *pvParameters)
{
    ESP_LOGI(TAG, "Starting System Task on Core %d", xPortGetCoreID());

    // Initialize WiFi (non-blocking process start)
    wifi_init_sta();

    ESP_LOGI(TAG, "System Task: Entering management loop.");
    uint32_t notifications = 0;
    bool last_wifi_connected = false;

    // Maintenance loop
    while (1)
    {
        // Wait for notifications or 1s timeout for basic maintenance
        xTaskNotifyWait(0, 0xFFFFFFFF, &notifications, pdMS_TO_TICKS(1000));

        // Check WiFi status and update global state
        g_state.wifi_connected = is_wifi_connected();

        if (g_state.wifi_connected && !last_wifi_connected)
        {
            ESP_LOGI(TAG, "WiFi connected: enabling Core 1 worker.");
            if (start_core1_task() && !g_state.should_stop && g_state.current_job.job_id != 0)
            {
                g_state.job_active = true;
                xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_JOB_LEASED, eSetBits);
            }
        }
        else if (!g_state.wifi_connected && last_wifi_connected)
        {
            ESP_LOGW(TAG, "WiFi disconnected: disabling Core 1 worker.");

            if (g_state.job_active && g_state.current_job.job_id != 0)
            {
                uint64_t current = atomic_load(&g_state.current_nonce);
                uint64_t scanned = atomic_load(&g_state.keys_scanned);

                job_checkpoint_t cp = {0};
                cp.job_id = g_state.current_job.job_id;
                memcpy(cp.prefix_28, g_state.current_job.prefix_28, PREFIX_28_SIZE);
                cp.nonce_start = g_state.current_job.nonce_start;
                cp.nonce_end = g_state.current_job.nonce_end;
                cp.current_nonce = current;
                cp.keys_scanned = scanned;
                cp.timestamp = (uint64_t)time(NULL);
                cp.magic = 0xACE1;

                esp_err_t cp_err = save_checkpoint(g_state.nvs_handle, &cp);
                if (cp_err != ESP_OK)
                {
                    ESP_LOGE(TAG, "Failed to save checkpoint on WiFi disconnect: %s", esp_err_to_name(cp_err));
                }
            }

            stop_core1_task();
        }

        last_wifi_connected = g_state.wifi_connected;

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
                    uint64_t duration = (esp_timer_get_time() / 1000) - atomic_load(&g_state.batch_start_ms);
                    esp_err_t api_err = api_checkpoint(cp.job_id, g_state.worker_id, current, scanned, duration);

                    if (api_err == ESP_ERR_INVALID_STATE)
                    {
                        ESP_LOGE(TAG, "Job %lld rejected by server (404/410). Stopping.", cp.job_id);
                        g_state.job_active = false;
                        g_state.current_job.job_id = 0;
                        nvs_clear_checkpoint(g_state.nvs_handle);
                        // Tell worker to abort immediately
                        if (g_state.core1_task_handle != NULL)
                        {
                            xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_STOP_SCAN, eSetBits);
                        }
                    }
                    else
                    {
                        // Signal worker that checkpoint is confirmed or ignored due to network error
                        if (g_state.core1_task_handle != NULL)
                        {
                            xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_CHECKPOINT_ACK, eSetBits);
                        }
                    }
                }
                else
                {
                    // No WiFi: just ACK locally so worker continues
                    if (g_state.core1_task_handle != NULL)
                    {
                        xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_CHECKPOINT_ACK, eSetBits);
                    }
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
                uint64_t duration = (esp_timer_get_time() / 1000) - atomic_load(&g_state.batch_start_ms);
                api_complete(g_state.current_job.job_id, g_state.worker_id, current, scanned, duration);
            }

            // Clear job information AFTER reporting to API to avoid reporting ID 0
            g_state.current_job.job_id = 0;
            atomic_store(&g_state.current_nonce, 0);
            atomic_store(&g_state.keys_scanned, 0);

            // Clear NVS checkpoint so we don't try to resume a finished job on reboot
            nvs_clear_checkpoint(g_state.nvs_handle);
        }

        // Handle Result Found Signal
        if (notifications & NOTIFY_BIT_RESULT_FOUND)
        {
            ESP_LOGI(TAG, "!!! MATCH FOUND Signal received from Core 1 !!!");

            // Clear checkpoint to prevent resuming an already handled match
            nvs_clear_checkpoint(g_state.nvs_handle);
            g_state.current_job.job_id = 0;

            found_result_t res;
            while (xQueueReceive(g_state.found_results_queue, &res, 0) == pdTRUE)
            {
                ESP_LOGI(TAG, "Processing result from queue for job %lld", res.job_id);

                if (g_state.wifi_connected)
                {
                    uint8_t derived_addr[20];
                    derive_eth_address(res.private_key, derived_addr);
                    api_submit_result(res.job_id, g_state.worker_id, res.private_key, derived_addr, res.nonce_found);
                }
                else
                {
                    ESP_LOGW(TAG, "Match found for job %lld but WiFi disconnected. Result dropped (not persisted in MVP).", res.job_id);
                }
            }
        }

        if (g_state.should_stop)
        {
            // Worker is in "Stop" state (Result found or shutdown), prevent leasing
            // We don't exit the loop so we can still respond to WiFi status or final signals
            vTaskDelay(pdMS_TO_TICKS(100));
            continue;
        }

        // P08-T120: Check if we just recovered a job from NVS and activate it immediately (even offline)
        if (g_state.wifi_connected && !g_state.job_active && !g_state.should_stop && g_state.current_job.job_id != 0)
        {
            ESP_LOGI(TAG, "RECOVERY: Activating recovered job %lld from nonce %llu (Initial Status: Offline-ready)",
                     g_state.current_job.job_id, (unsigned long long)atomic_load(&g_state.current_nonce));
            atomic_store(&g_state.batch_start_ms, esp_timer_get_time() / 1000);
            g_state.job_active = true;
            if (g_state.core1_task_handle != NULL)
            {
                xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_JOB_LEASED, eSetBits);
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
                atomic_store(&g_state.batch_start_ms, esp_timer_get_time() / 1000);
                g_state.job_active = true;

                // Create initial checkpoint to allow recovery if we crash shortly after leasing
                job_checkpoint_t cp = {0};
                cp.job_id = new_job.job_id;
                memcpy(cp.prefix_28, new_job.prefix_28, PREFIX_28_SIZE);
                cp.nonce_start = new_job.nonce_start;
                cp.nonce_end = new_job.nonce_end;
                cp.current_nonce = new_job.nonce_start;
                cp.keys_scanned = 0;
                cp.timestamp = (uint64_t)time(NULL);
                cp.magic = 0xACE1;
                save_checkpoint(g_state.nvs_handle, &cp);

                // Signal Core 1 task to start working
                if (g_state.core1_task_handle != NULL)
                {
                    xTaskNotify(g_state.core1_task_handle, NOTIFY_BIT_JOB_LEASED, eSetBits);
                }
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
    ESP_LOGI(TAG, "Core 1: Worker task started (WiFi already connected).");
    vTaskPrioritySet(NULL, configMAX_PRIORITIES - 2);

    ESP_LOGI(TAG, "Core 1: Worker state machine active (Waiting for jobs).");
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
                set_led_status(LED_SCANNING);

                // Initialize job-specific state
                memcpy(priv_key, g_state.current_job.prefix_28, 28);

                // P08-T120: Load from atomic current_nonce for recovery support
                uint32_t current = (uint32_t)atomic_load(&g_state.current_nonce);
                uint32_t end = (uint32_t)g_state.current_job.nonce_end;
                uint32_t start = (uint32_t)g_state.current_job.nonce_start;
                uint32_t total = (end >= start) ? (end - start + 1) : 1;

                uint32_t session_scanned = 0;
                uint32_t throughput = g_state.stats.keys_per_second;

                // Base mask for LED activity - adjust to maintain visibility at any speed
                uint32_t base_pulse_mask = 0x3F; // Default for slow devices (<100 keys/sec) -> ~1.5s interval
                if (throughput > 2000)
                    base_pulse_mask = 0xFFF;
                else if (throughput > 500)
                    base_pulse_mask = 0x3FF;
                else if (throughput > 100)
                    base_pulse_mask = 0xFF;

                ESP_LOGI(TAG, "Core 1: Scan loop starting (Throughput: %lu, Mask: 0x%lx, Range: %lu -> %lu, Total: %lu)",
                         (unsigned long)throughput, (unsigned long)base_pulse_mask,
                         (unsigned long)start, (unsigned long)end, (unsigned long)total);

                while (g_state.job_active && !g_state.should_stop)
                {
                    if (current > end)
                    {
                        ESP_LOGI(TAG, "Core 1: Job range completed successfully.");
                        set_led_status(LED_WIFI_CONNECTED);
                        xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_JOB_COMPLETE, eSetBits);
                        break;
                    }

                    uint32_t progress = (current >= start) ? (current - start) : 0;

                    // Optimized byte-level nonce manipulation (P08-T080)
                    update_nonce_in_buffer(priv_key, current);

                    // Derive Ethereum address from the current private key (P08-T100)
                    uint8_t derived_addr[20];
                    derive_eth_address(priv_key, derived_addr);

                    // Binary comparison using memcmp for zero-overhead validation (P08-T090)
                    bool match = false;
                    for (int i = 0; i < g_state.current_job.num_targets; i++)
                    {
                        if (memcmp(derived_addr, g_state.current_job.target_addresses[i], 20) == 0)
                        {
                            match = true;
                            break;
                        }
                    }
                    if (match)
                    {
                        ESP_LOGI(TAG, "Core 1: !!! MATCH FOUND !!! at nonce %lu", (unsigned long)current);
                        set_led_status(LED_KEY_FOUND);

                        found_result_t res;
                        res.job_id = g_state.current_job.job_id;
                        res.nonce_found = current;
                        memcpy(res.private_key, priv_key, 32);

                        if (xQueueSend(g_state.found_results_queue, &res, 0) == pdTRUE)
                        {
                            xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_RESULT_FOUND, eSetBits);
                        }
                        else
                        {
                            ESP_LOGE(TAG, "Core 1: FAILED TO QUEUE RESULT! Queue full.");
                        }

                        // Stop everything: deactivate job and stop worker loop
                        g_state.job_active = false;
                        g_state.should_stop = true;
                        break;
                    }

                    // Increment and update global progress
                    current++;
                    session_scanned++;
                    atomic_fetch_add(&g_state.current_nonce, 1);
                    atomic_fetch_add(&g_state.keys_scanned, 1);

                    // Progressive LED feedback: pulse faster as we approach the end
                    uint32_t pulse_mask = base_pulse_mask;

                    if (progress > (total * 9) / 10)
                        pulse_mask = (base_pulse_mask >> 3) | 1; // Ultra speed
                    else if (progress > (total * 3) / 4)
                        pulse_mask = (base_pulse_mask >> 2) | 1; // Fast
                    else if (progress > total / 2)
                        pulse_mask = (base_pulse_mask >> 1) | 1; // Medium-fast

                    if ((progress & pulse_mask) == 0)
                    {
                        led_trigger_activity();
                    }

                    // Progress Logging & Mandatory Checkpoint (every 2500 keys)
                    if (progress > 0 && (progress % 2500 == 0))
                    {
                        uint32_t percent = total > 0 ? (progress * 100 / total) : 0;
                        ESP_LOGI(TAG, "Core 1 Progress: %lu/%lu keys (%lu%%) | Nonce: %lu | Session: +%lu",
                                 (unsigned long)progress, (unsigned long)total,
                                 (unsigned long)percent, (unsigned long)current,
                                 (unsigned long)session_scanned);

                        // Mandatory synchronous checkpoint - don't continue until Master acknowledges
                        xTaskNotify(g_state.core0_task_handle, NOTIFY_BIT_CHECKPOINT, eSetBits);

                        // Wait for Core 0 to finish checkpointing
                        uint32_t ack_notif = 0;
                        if (xTaskNotifyWait(0, 0xFFFFFFFF, &ack_notif, pdMS_TO_TICKS(10000)) == pdTRUE)
                        {
                            if (ack_notif & NOTIFY_BIT_STOP_SCAN)
                            {
                                ESP_LOGE(TAG, "Core 1: Fatal checkpoint error. Stopping scan.");
                                break;
                            }
                            // Continue scanning on ACK
                        }
                        else
                        {
                            ESP_LOGW(TAG, "Core 1: Checkpoint ACK timeout. Carrying on...");
                        }
                    }

                    // Frequently yield to allow system tasks and IDLE to reset WDT.
                    if ((progress & 0x7F) == 0)
                    {
                        vTaskDelay(1);
                        // Also check for STOP_SCAN signal between yields
                        uint32_t async_notif = 0;
                        if (xTaskNotifyWait(0, 0xFFFFFFFF, &async_notif, 0) == pdTRUE)
                        {
                            if (async_notif & NOTIFY_BIT_STOP_SCAN)
                            {
                                ESP_LOGE(TAG, "Core 1: External STOP signal received.");
                                break;
                            }
                        }
                    }
                }
            }

            // Feed the watchdog by yielding when idle
            vTaskDelay(pdMS_TO_TICKS(100));
        }
    }
}