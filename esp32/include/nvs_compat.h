#ifndef NVS_COMPAT_H
#define NVS_COMPAT_H

#include "esp_err.h"
#include "nvs.h"

// Wrappers that call real NVS functions in production but can be overridden in tests.
esp_err_t nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle);
esp_err_t nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats);
esp_err_t nvs_flash_init_wr(void);
esp_err_t nvs_flash_erase_wr(void);
esp_err_t nvs_set_blob_wr(nvs_handle_t handle, const char *key, const void *value, size_t length);
esp_err_t nvs_get_blob_wr(nvs_handle_t handle, const char *key, void *out_value, size_t *length);
esp_err_t nvs_erase_key_wr(nvs_handle_t handle, const char *key);
esp_err_t nvs_commit_wr(nvs_handle_t handle);

#endif // NVS_COMPAT_H
