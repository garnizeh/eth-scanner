#ifndef NVS_COMPAT_H
#define NVS_COMPAT_H

#include "esp_err.h"
#include "nvs.h"

// Wrappers that call real NVS functions in production but can be overridden in tests.
esp_err_t nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle);
esp_err_t nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats);
esp_err_t nvs_flash_init_wr(void);
esp_err_t nvs_flash_erase_wr(void);

#endif // NVS_COMPAT_H
