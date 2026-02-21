#include "nvs_compat.h"
#include "nvs_flash.h"
#include "nvs.h"

// Weak wrapper definitions that call the real NVS functions by default.
// Mark the definitions as weak so the strong stub implementations in
// `test_stubs.c` can provide overrides during unit tests.

esp_err_t __attribute__((weak)) nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle)
{
    return nvs_open(name, open_mode, out_handle);
}

esp_err_t __attribute__((weak)) nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats)
{
    return nvs_get_stats(partition_name, stats);
}

esp_err_t __attribute__((weak)) nvs_flash_init_wr(void)
{
    return nvs_flash_init();
}

esp_err_t __attribute__((weak)) nvs_flash_erase_wr(void)
{
    return nvs_flash_erase();
}

esp_err_t __attribute__((weak)) nvs_set_blob_wr(nvs_handle_t handle, const char *key, const void *value, size_t length)
{
    return nvs_set_blob(handle, key, value, length);
}

esp_err_t __attribute__((weak)) nvs_get_blob_wr(nvs_handle_t handle, const char *key, void *out_value, size_t *length)
{
    return nvs_get_blob(handle, key, out_value, length);
}

esp_err_t __attribute__((weak)) nvs_erase_key_wr(nvs_handle_t handle, const char *key)
{
    return nvs_erase_key(handle, key);
}

esp_err_t __attribute__((weak)) nvs_commit_wr(nvs_handle_t handle)
{
    return nvs_commit(handle);
}

// Weak wrappers for HTTP client functions.
esp_http_client_handle_t __attribute__((weak)) esp_http_client_init_wr(const esp_http_client_config_t *config)
{
    return esp_http_client_init(config);
}

esp_err_t __attribute__((weak)) esp_http_client_cleanup_wr(esp_http_client_handle_t client)
{
    return esp_http_client_cleanup(client);
}

esp_err_t __attribute__((weak)) esp_http_client_perform_wr(esp_http_client_handle_t client)
{
    return esp_http_client_perform(client);
}

int __attribute__((weak)) esp_http_client_get_status_code_wr(esp_http_client_handle_t client)
{
    return esp_http_client_get_status_code(client);
}

esp_err_t __attribute__((weak)) esp_http_client_set_header_wr(esp_http_client_handle_t client, const char *field, const char *value)
{
    return esp_http_client_set_header(client, field, value);
}

esp_err_t __attribute__((weak)) esp_http_client_set_post_field_wr(esp_http_client_handle_t client, const char *data, int len)
{
    return esp_http_client_set_post_field(client, data, len);
}
