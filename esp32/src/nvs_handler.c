#include "esp_log.h"
#include "nvs_handler.h"
#include "global_state.h"
#include "nvs_compat.h"

static const char *TAG = "nvs-handler";

global_state_t g_state = {0};

esp_err_t nvs_handler_init(void)
{
    esp_err_t err;

    // Open the "storage" namespace with read/write access
    err = nvs_open_wr("storage", NVS_READWRITE, &g_state.nvs_handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error opening NVS namespace 'storage': %s", esp_err_to_name(err));
        return err;
    }

    // Log NVS stats
    nvs_stats_t stats;
    err = nvs_get_stats_wr(NULL, &stats);
    if (err == ESP_OK) {
        ESP_LOGI(TAG, "NVS - Used: %d, Free: %d, Total: %d",
                 stats.used_entries, stats.free_entries, stats.total_entries);
    } else {
        ESP_LOGW(TAG, "Failed to get NVS stats: %s", esp_err_to_name(err));
    }

    ESP_LOGI(TAG, "NVS namespace 'storage' opened successfully");
    return ESP_OK;
}
