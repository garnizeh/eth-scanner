#include "esp_log.h"
#include "esp_timer.h"
#include "nvs_handler.h"
#include "global_state.h"
#include "nvs_compat.h"
#include <string.h>

static const char *TAG = "nvs-handler";

#define NVS_CHECKPOINT_KEY "job_ckpt"
#define CHECKPOINT_MAGIC 0xDEADBEEF

global_state_t g_state = {0};

esp_err_t nvs_handler_init(void)
{
    esp_err_t err;

    // Open the "storage" namespace with read/write access
    err = nvs_open_wr("storage", NVS_READWRITE, &g_state.nvs_handle);
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "Error opening NVS namespace 'storage': %s", esp_err_to_name(err));
        return err;
    }

    // Log NVS stats
    nvs_stats_t stats;
    err = nvs_get_stats_wr(NULL, &stats);
    if (err == ESP_OK)
    {
        ESP_LOGI(TAG, "NVS - Used: %d, Free: %d, Total: %d",
                 stats.used_entries, stats.free_entries, stats.total_entries);
    }
    else
    {
        ESP_LOGW(TAG, "Failed to get NVS stats: %s", esp_err_to_name(err));
    }

    ESP_LOGI(TAG, "NVS namespace 'storage' opened successfully");
    return ESP_OK;
}

esp_err_t save_checkpoint(nvs_handle_t handle, const job_checkpoint_t *checkpoint)
{
    if (checkpoint == NULL)
    {
        return ESP_ERR_INVALID_ARG;
    }

    // Set magic number and timestamp for validity check
    job_checkpoint_t ckpt_copy = *checkpoint;
    ckpt_copy.magic = CHECKPOINT_MAGIC;
    ckpt_copy.timestamp = esp_timer_get_time() / 1000000ULL; // seconds since boot

    // Write blob atomically using wrapper
    esp_err_t err = nvs_set_blob_wr(handle, NVS_CHECKPOINT_KEY,
                                    &ckpt_copy, sizeof(job_checkpoint_t));
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "Failed to write checkpoint to NVS: %s", esp_err_to_name(err));
        return err;
    }

    // Commit to ensure data is written to flash
    err = nvs_commit_wr(handle);
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "Failed to commit NVS write: %s", esp_err_to_name(err));
        return err;
    }

    ESP_LOGI(TAG, "Checkpoint saved: job_id=%lld, current_nonce=%llu",
             ckpt_copy.job_id, (unsigned long long)ckpt_copy.current_nonce);

    return ESP_OK;
}
#define CHECKPOINT_MAX_AGE_SEC (3600 * 2) // 2 hours staleness limit

esp_err_t load_checkpoint(nvs_handle_t handle, job_checkpoint_t *out_checkpoint)
{
    if (out_checkpoint == NULL)
    {
        return ESP_ERR_INVALID_ARG;
    }

    size_t required_size = sizeof(job_checkpoint_t);
    esp_err_t err = nvs_get_blob_wr(handle, NVS_CHECKPOINT_KEY,
                                    out_checkpoint, &required_size);

    if (err == ESP_ERR_NVS_NOT_FOUND)
    {
        ESP_LOGI(TAG, "No checkpoint found in NVS");
        return ESP_ERR_NOT_FOUND;
    }
    else if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "Error reading checkpoint: %s", esp_err_to_name(err));
        return err;
    }

    // Validate magic number
    if (out_checkpoint->magic != CHECKPOINT_MAGIC)
    {
        ESP_LOGW(TAG, "Invalid checkpoint magic: 0x%08X", (unsigned int)out_checkpoint->magic);
        return ESP_ERR_INVALID_CRC;
    }

    // Check staleness (prevent resuming very old jobs)
    uint64_t now = esp_timer_get_time() / 1000000ULL;
    if (out_checkpoint->timestamp > now || (now - out_checkpoint->timestamp) > CHECKPOINT_MAX_AGE_SEC)
    {
        uint64_t age = (out_checkpoint->timestamp > now) ? 0 : (now - out_checkpoint->timestamp);
        ESP_LOGW(TAG, "Checkpoint stale or from future (age: %llu sec, now: %llu, ts: %llu), discarding",
                 (unsigned long long)age, (unsigned long long)now, (unsigned long long)out_checkpoint->timestamp);
        return ESP_ERR_INVALID_STATE;
    }

    ESP_LOGI(TAG, "Checkpoint loaded: job_id=%lld, current_nonce=%llu",
             out_checkpoint->job_id, (unsigned long long)out_checkpoint->current_nonce);

    return ESP_OK;
}