#ifndef NVS_COMPAT_H
#define NVS_COMPAT_H

#include "esp_err.h"
#include "nvs.h"
#include "esp_http_client.h"

// Wrappers that call real NVS functions in production but can be overridden in tests.
esp_err_t nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle);
esp_err_t nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats);
esp_err_t nvs_flash_init_wr(void);
esp_err_t nvs_flash_erase_wr(void);
esp_err_t nvs_set_blob_wr(nvs_handle_t handle, const char *key, const void *value, size_t length);
esp_err_t nvs_get_blob_wr(nvs_handle_t handle, const char *key, void *out_value, size_t *length);
esp_err_t nvs_erase_key_wr(nvs_handle_t handle, const char *key);
esp_err_t nvs_commit_wr(nvs_handle_t handle);

// Wrappers for HTTP client functions to facilitate unit testing.
esp_http_client_handle_t esp_http_client_init_wr(const esp_http_client_config_t *config);
esp_err_t esp_http_client_cleanup_wr(esp_http_client_handle_t client);
esp_err_t esp_http_client_perform_wr(esp_http_client_handle_t client);
int esp_http_client_get_status_code_wr(esp_http_client_handle_t client);
esp_err_t esp_http_client_set_header_wr(esp_http_client_handle_t client, const char *field, const char *value);
esp_err_t esp_http_client_set_post_field_wr(esp_http_client_handle_t client, const char *data, int len);

#endif // NVS_COMPAT_H
