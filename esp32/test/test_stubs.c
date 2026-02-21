// Controllable stubs for NVS behavior used by unit tests. These define
// the wrapper symbols (nvs_*_wr) so they do not conflict with the real
// nvs_flash library symbols when linking.
#include "nvs_compat.h"
#include "shared_types.h"
#include "esp_http_client.h"
#include <string.h>

int stub_nvs_open_behavior = 0; // 0 = success, 1 = open error
int stub_nvs_stats_error = 0;   // 0 = success, 1 = error
int use_nvs_flash_sequence = 0;
int nvs_flash_init_call_count = 0;
esp_err_t nvs_flash_sequence[8];
int nvs_flash_sequence_len = 0;
int nvs_erase_count = 0;

esp_err_t nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle)
{
    if (stub_nvs_open_behavior == 1)
    {
        return ESP_ERR_NO_MEM;
    }
    *out_handle = (nvs_handle_t)0xDEADBEEF;
    return ESP_OK;
}

esp_err_t nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats)
{
    if (stub_nvs_stats_error)
    {
        return ESP_ERR_NOT_FOUND;
    }
    if (stats)
    {
        stats->used_entries = 1;
        stats->free_entries = 2;
        stats->total_entries = 3;
    }
    return ESP_OK;
}

esp_err_t nvs_flash_init_wr(void)
{
    nvs_flash_init_call_count++;
    if (use_nvs_flash_sequence && nvs_flash_init_call_count <= nvs_flash_sequence_len)
    {
        return nvs_flash_sequence[nvs_flash_init_call_count - 1];
    }
    return ESP_OK;
}

esp_err_t nvs_flash_erase_wr(void)
{
    nvs_erase_count++;
    return ESP_OK;
}

// Memory-based NVS blob for testing
uint8_t g_test_nvs_blob[512];
size_t g_test_nvs_blob_len = 0;
int stub_nvs_set_blob_error = 0;
int stub_nvs_commit_error = 0;
int nvs_commit_count = 0;

esp_err_t nvs_set_blob_wr(nvs_handle_t handle, const char *key, const void *value, size_t length)
{
    if (stub_nvs_set_blob_error)
        return ESP_ERR_NVS_NOT_ENOUGH_SPACE;
    if (length > sizeof(g_test_nvs_blob))
        return ESP_ERR_NVS_VALUE_TOO_LONG;
    memcpy(g_test_nvs_blob, value, length);
    g_test_nvs_blob_len = length;
    return ESP_OK;
}

esp_err_t nvs_get_blob_wr(nvs_handle_t handle, const char *key, void *out_value, size_t *length)
{
    if (g_test_nvs_blob_len == 0)
        return ESP_ERR_NVS_NOT_FOUND;
    if (out_value == NULL)
    {
        *length = g_test_nvs_blob_len;
        return ESP_OK;
    }
    if (*length < g_test_nvs_blob_len)
        return ESP_ERR_NVS_INVALID_LENGTH;
    memcpy(out_value, g_test_nvs_blob, g_test_nvs_blob_len);
    *length = g_test_nvs_blob_len;
    return ESP_OK;
}

esp_err_t nvs_commit_wr(nvs_handle_t handle)
{
    if (stub_nvs_commit_error)
        return ESP_ERR_NVS_NOT_FOUND;
    nvs_commit_count++;
    return ESP_OK;
}

esp_err_t nvs_erase_key_wr(nvs_handle_t handle, const char *key)
{
    if (strcmp(key, "job_ckpt") == 0)
    {
        g_test_nvs_blob_len = 0;
    }
    return ESP_OK;
}

// Simple struct so we can store callback/user data in mocks
typedef struct esp_http_client_mock
{
    void *user_data;
    esp_err_t (*event_handler)(esp_http_client_event_t *evt);
} esp_http_client_mock_t;

esp_http_client_handle_t esp_http_client_init_wr(const esp_http_client_config_t *config)
{
    esp_http_client_mock_t *c = malloc(sizeof(esp_http_client_mock_t));
    if (c)
    {
        c->user_data = config->user_data;
        c->event_handler = config->event_handler;
    }
    return (esp_http_client_handle_t)c;
}

esp_err_t esp_http_client_cleanup_wr(esp_http_client_handle_t client1)
{
    free(client1);
    return ESP_OK;
}

esp_err_t esp_http_client_set_header_wr(esp_http_client_handle_t client1, const char *field, const char *value)
{
    return ESP_OK;
}

esp_err_t esp_http_client_set_post_field_wr(esp_http_client_handle_t client1, const char *data, int len)
{
    return ESP_OK;
}

// Memory-based cJSON response stub for networking tests
static char g_mock_http_response[2048];
int g_mock_http_status = 200;

void set_mock_http_response(int status, const char *json_body)
{
    g_mock_http_status = status;
    if (json_body)
    {
        strncpy(g_mock_http_response, json_body, sizeof(g_mock_http_response) - 1);
        g_mock_http_response[sizeof(g_mock_http_response) - 1] = '\0';
    }
    else
    {
        g_mock_http_response[0] = '\0';
    }
}

esp_err_t esp_http_client_perform_wr(esp_http_client_handle_t client1)
{
    esp_http_client_mock_t *c = (esp_http_client_mock_t *)client1;

    // Simulate callback if handler exists and status 200
    if (g_mock_http_status == 200 && c && c->event_handler && g_mock_http_response[0] != '\0')
    {
        esp_http_client_event_t evt = {
            .event_id = HTTP_EVENT_ON_DATA,
            .user_data = c->user_data,
            .data = g_mock_http_response,
            .data_len = strlen(g_mock_http_response)};
        c->event_handler(&evt);
    }
    return ESP_OK;
}

int esp_http_client_get_status_code_wr(esp_http_client_handle_t client1)
{
    return g_mock_http_status;
}
