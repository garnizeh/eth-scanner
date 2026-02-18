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
