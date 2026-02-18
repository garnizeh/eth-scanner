// Controllable stubs for NVS behavior used by unit tests. These define
// the wrapper symbols (nvs_*_wr) so they do not conflict with the real
// nvs_flash library symbols when linking.
#include "nvs_compat.h"

int stub_nvs_open_behavior = 0; // 0 = success, 1 = open error
int stub_nvs_stats_error = 0;   // 0 = success, 1 = error
int use_nvs_flash_sequence = 0;
int nvs_flash_init_call_count = 0;
esp_err_t nvs_flash_sequence[8];
int nvs_flash_sequence_len = 0;
int nvs_erase_count = 0;

esp_err_t nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle)
{
    if (stub_nvs_open_behavior == 1) {
        return ESP_ERR_NO_MEM;
    }
    *out_handle = (nvs_handle_t)0xDEADBEEF;
    return ESP_OK;
}

esp_err_t nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats)
{
    if (stub_nvs_stats_error) {
        return ESP_ERR_NOT_FOUND;
    }
    if (stats) {
        stats->used_entries = 1;
        stats->free_entries = 2;
        stats->total_entries = 3;
    }
    return ESP_OK;
}

esp_err_t nvs_flash_init_wr(void)
{
    nvs_flash_init_call_count++;
    if (use_nvs_flash_sequence && nvs_flash_init_call_count <= nvs_flash_sequence_len) {
        return nvs_flash_sequence[nvs_flash_init_call_count - 1];
    }
    return ESP_OK;
}

esp_err_t nvs_flash_erase_wr(void)
{
    nvs_erase_count++;
    return ESP_OK;
}
