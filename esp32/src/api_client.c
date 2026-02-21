#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "esp_http_client.h"
#include "cJSON.h"
#include "mbedtls/base64.h"
#include "api_client.h"
#include "sdkconfig.h"
#include "nvs_compat.h"

static const char *TAG = "api_client";

// Maximum response buffer size (increased to 8KB for safety with many target addresses)
#define MAX_HTTP_RECV_BUFFER 8192

typedef struct
{
    char *buffer;
    int buffer_len;
} response_data_t;

static int hex_to_int(char c)
{
    if (c >= '0' && c <= '9')
        return c - '0';
    if (c >= 'a' && c <= 'f')
        return c - 'a' + 10;
    if (c >= 'A' && c <= 'F')
        return c - 'A' + 10;
    return -1;
}

static void hex_to_bytes(const char *hex, uint8_t *bytes, size_t len)
{
    if (hex[0] == '0' && hex[1] == 'x')
        hex += 2;
    for (size_t i = 0; i < len; i++)
    {
        bytes[i] = (hex_to_int(hex[i * 2]) << 4) | hex_to_int(hex[i * 2 + 1]);
    }
}

/**
 * @brief Handle HTTP events and capture response body
 */
static esp_err_t http_event_handler(esp_http_client_event_t *evt)
{
    response_data_t *res = (response_data_t *)evt->user_data;
    switch (evt->event_id)
    {
    case HTTP_EVENT_ON_DATA:
        if (res && res->buffer && (res->buffer_len + evt->data_len < MAX_HTTP_RECV_BUFFER))
        {
            memcpy(res->buffer + res->buffer_len, evt->data, evt->data_len);
            res->buffer_len += evt->data_len;
            res->buffer[res->buffer_len] = '\0';
        }
        break;
    default:
        break;
    }
    return ESP_OK;
}

esp_err_t api_lease_job(const char *worker_id, uint32_t batch_size,
                        job_info_t *out_job)
{
    const char *url = CONFIG_ETHSCANNER_API_URL "/api/v1/jobs/lease";
    ESP_LOGI(TAG, "Requesting lease for worker: %s (URL: %s)", worker_id, url);

    // Use heap for large response buffer instead of stack (prevent overflow on worker tasks)
    char *response_buffer = (char *)malloc(MAX_HTTP_RECV_BUFFER);
    if (!response_buffer)
    {
        ESP_LOGE(TAG, "Failed to allocate memory for HTTP response buffer");
        return ESP_ERR_NO_MEM;
    }
    memset(response_buffer, 0, MAX_HTTP_RECV_BUFFER);

    response_data_t res = {
        .buffer = response_buffer,
        .buffer_len = 0};

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .event_handler = http_event_handler,
        .user_data = &res,
        .timeout_ms = 5000,
    };

    esp_http_client_handle_t client = esp_http_client_init_wr(&config);
    if (client == NULL)
    {
        ESP_LOGE(TAG, "Failed to initialize HTTP client");
        free(response_buffer);
        return ESP_FAIL;
    }

    // Build JSON request body
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "worker_id", worker_id);
    cJSON_AddStringToObject(root, "worker_type", "esp32");
    cJSON_AddNumberToObject(root, "requested_batch_size", (double)batch_size);

    char *json_str = cJSON_PrintUnformatted(root);

    esp_http_client_set_header_wr(client, "Content-Type", "application/json");
    if (json_str)
    {
        esp_http_client_set_post_field_wr(client, json_str, strlen(json_str));
    }

    esp_err_t err = esp_http_client_perform_wr(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code_wr(client);
        if (status == 200)
        {
            cJSON *resp_json = cJSON_Parse(response_buffer);
            if (resp_json)
            {
                cJSON *item = cJSON_GetObjectItem(resp_json, "job_id");
                if (item && cJSON_IsNumber(item))
                    out_job->job_id = (int64_t)item->valuedouble;

                item = cJSON_GetObjectItem(resp_json, "nonce_start");
                if (item && cJSON_IsNumber(item))
                    out_job->nonce_start = (uint64_t)item->valuedouble;

                item = cJSON_GetObjectItem(resp_json, "nonce_end");
                if (item && cJSON_IsNumber(item))
                    out_job->nonce_end = (uint64_t)item->valuedouble;

                // Load target addresses
                out_job->num_targets = 0;
                const cJSON *targets = cJSON_GetObjectItem(resp_json, "target_addresses");
                if (cJSON_IsArray(targets))
                {
                    int size = cJSON_GetArraySize(targets);
                    if (size > MAX_TARGET_ADDRESSES)
                        size = MAX_TARGET_ADDRESSES;
                    for (int i = 0; i < size; i++)
                    {
                        const cJSON *target_ptr = cJSON_GetArrayItem(targets, i);
                        if (cJSON_IsString(target_ptr))
                        {
                            hex_to_bytes(target_ptr->valuestring, out_job->target_addresses[out_job->num_targets++], 20);
                        }
                    }
                }

                cJSON *prefix_item = cJSON_GetObjectItem(resp_json, "prefix_28");
                if (prefix_item && cJSON_IsString(prefix_item))
                {
                    const char *prefix_b64 = prefix_item->valuestring;
                    size_t olen = 0;
                    int decode_ret = mbedtls_base64_decode(out_job->prefix_28, sizeof(out_job->prefix_28), &olen,
                                                           (const unsigned char *)prefix_b64, strlen(prefix_b64));

                    if (decode_ret != 0 || olen != 28)
                    {
                        ESP_LOGE(TAG, "Failed to decode prefix_28: %d (len=%d)", decode_ret, (int)olen);
                        err = ESP_FAIL;
                    }
                }
                else
                {
                    ESP_LOGE(TAG, "Missing prefix_28 in lease response");
                    err = ESP_FAIL;
                }
                cJSON_Delete(resp_json);
            }
            else
            {
                ESP_LOGE(TAG, "Failed to parse lease response JSON");
                err = ESP_FAIL;
            }
        }
        else if (status == 404)
        {
            ESP_LOGW(TAG, "No jobs available (404)");
            err = ESP_ERR_NOT_FOUND;
        }
        else
        {
            ESP_LOGE(TAG, "Lease request failed with HTTP status %d", status);
            err = ESP_FAIL;
        }
    }
    else
    {
        ESP_LOGE(TAG, "Lease request performance failed: %s", esp_err_to_name(err));
    }

    cJSON_Delete(root);
    if (json_str)
        free(json_str);
    esp_http_client_cleanup_wr(client);
    free(response_buffer);

    return err;
}

esp_err_t api_checkpoint(int64_t job_id, const char *worker_id,
                         uint64_t current_nonce, uint64_t keys_scanned,
                         uint64_t duration_ms)
{
    char url[256];
    snprintf(url, sizeof(url), "%s/api/v1/jobs/%lld/checkpoint", CONFIG_ETHSCANNER_API_URL, job_id);
    ESP_LOGI(TAG, "Sending checkpoint for job %lld to %s", job_id, url);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_PATCH,
        .timeout_ms = 5000,
    };

    esp_http_client_handle_t client = esp_http_client_init_wr(&config);
    if (client == NULL)
    {
        ESP_LOGE(TAG, "Failed to initialize HTTP client for checkpoint");
        return ESP_FAIL;
    }

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "worker_id", worker_id);
    cJSON_AddNumberToObject(root, "current_nonce", (double)current_nonce);
    cJSON_AddNumberToObject(root, "keys_scanned", (double)keys_scanned);
    cJSON_AddNumberToObject(root, "duration_ms", (double)duration_ms);

    char *json_str = cJSON_PrintUnformatted(root);
    if (!json_str)
    {
        cJSON_Delete(root);
        esp_http_client_cleanup_wr(client);
        return ESP_FAIL;
    }

    esp_http_client_set_header_wr(client, "Content-Type", "application/json");
    esp_http_client_set_post_field_wr(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform_wr(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code_wr(client);
        if (status == 200)
        {
            // Success
        }
        else if (status == 404 || status == 410)
        {
            ESP_LOGW(TAG, "Checkpoint failed: Job %lld no longer valid on server (Status %d)", job_id, status);
            err = ESP_ERR_INVALID_STATE;
        }
        else
        {
            ESP_LOGE(TAG, "Checkpoint failed with HTTP status %d", status);
            err = ESP_FAIL;
        }
    }
    else
    {
        ESP_LOGE(TAG, "Checkpoint performance failed: %s", esp_err_to_name(err));
    }

    cJSON_Delete(root);
    free(json_str);
    esp_http_client_cleanup_wr(client);

    return err;
}

esp_err_t api_complete(int64_t job_id, const char *worker_id,
                       uint64_t final_nonce, uint64_t keys_scanned,
                       uint64_t duration_ms)
{
    char url[256];
    snprintf(url, sizeof(url), "%s/api/v1/jobs/%lld/complete", CONFIG_ETHSCANNER_API_URL, job_id);
    ESP_LOGI(TAG, "Completing job %lld (final_nonce: %u) (URL: %s)", job_id, (unsigned int)final_nonce, url);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .timeout_ms = 5000,
    };

    esp_http_client_handle_t client = esp_http_client_init_wr(&config);
    if (client == NULL)
    {
        ESP_LOGE(TAG, "Failed to initialize HTTP client for complete");
        return ESP_FAIL;
    }

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "worker_id", worker_id);
    cJSON_AddNumberToObject(root, "final_nonce", (double)final_nonce);
    cJSON_AddNumberToObject(root, "keys_scanned", (double)keys_scanned);
    cJSON_AddNumberToObject(root, "duration_ms", (double)duration_ms);

    char *json_str = cJSON_PrintUnformatted(root);
    if (!json_str)
    {
        cJSON_Delete(root);
        esp_http_client_cleanup_wr(client);
        return ESP_FAIL;
    }

    esp_http_client_set_header_wr(client, "Content-Type", "application/json");
    esp_http_client_set_post_field_wr(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform_wr(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code_wr(client);
        if (status == 200)
        {
            // Success
        }
        else if (status == 404 || status == 410)
        {
            ESP_LOGW(TAG, "Complete failed: Job %lld no longer valid on server (Status %d)", job_id, status);
            err = ESP_ERR_INVALID_STATE;
        }
        else
        {
            ESP_LOGE(TAG, "Complete failed with HTTP status %d", status);
            err = ESP_FAIL;
        }
    }
    else
    {
        ESP_LOGE(TAG, "Complete performance failed: %s", esp_err_to_name(err));
    }

    cJSON_Delete(root);
    free(json_str);
    esp_http_client_cleanup_wr(client);

    return err;
}

esp_err_t api_submit_result(int64_t job_id, const char *worker_id,
                            const uint8_t *private_key, const uint8_t *address,
                            uint64_t nonce)
{
    char url[256];
    snprintf(url, sizeof(url), "%s/api/v1/results", CONFIG_ETHSCANNER_API_URL);
    ESP_LOGI(TAG, "!!! MATCH FOUND !!! Submitting result for job %lld (nonce: %llu) to %s", job_id, (unsigned long long)nonce, url);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .timeout_ms = 10000, // Longer timeout for critical submission
    };

    esp_http_client_handle_t client = esp_http_client_init_wr(&config);
    if (client == NULL)
    {
        ESP_LOGE(TAG, "Failed to initialize HTTP client for result submission");
        return ESP_FAIL;
    }

    // Convert keys to hex strings for JSON (simple approach for ESP32)
    char priv_hex[65] = {0};
    char addr_hex[43] = {0};
    // Address must be 0x-prefixed for the Master API to accept it.
    strcpy(addr_hex, "0x");
    for (int i = 0; i < 32; i++)
        sprintf(priv_hex + (i * 2), "%02x", private_key[i]);
    for (int i = 0; i < 20; i++)
        sprintf(addr_hex + 2 + (i * 2), "%02x", address[i]);

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "worker_id", worker_id);
    cJSON_AddNumberToObject(root, "job_id", (double)job_id);
    cJSON_AddStringToObject(root, "private_key", priv_hex);
    cJSON_AddStringToObject(root, "address", addr_hex);
    cJSON_AddNumberToObject(root, "nonce", (double)nonce);

    char *json_str = cJSON_PrintUnformatted(root);
    if (!json_str)
    {
        cJSON_Delete(root);
        esp_http_client_cleanup_wr(client);
        return ESP_FAIL;
    }

    esp_http_client_set_header_wr(client, "Content-Type", "application/json");
    esp_http_client_set_post_field_wr(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform_wr(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code_wr(client);
        if (status != 200 && status != 201)
        {
            ESP_LOGE(TAG, "Result submission failed with HTTP status %d", status);
            err = ESP_FAIL;
        }
        else
        {
            ESP_LOGI(TAG, "Result submitted successfully!");
        }
    }
    else
    {
        ESP_LOGE(TAG, "Result submission performance failed: %s", esp_err_to_name(err));
    }

    cJSON_Delete(root);
    free(json_str);
    esp_http_client_cleanup_wr(client);

    return err;
}
