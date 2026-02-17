#include <stdio.h>
#include <string.h>
#include "esp_log.h"
#include "esp_http_client.h"
#include "cJSON.h"
#include "mbedtls/base64.h"
#include "api_client.h"
#include "sdkconfig.h"

static const char *TAG = "api_client";

// Maximum response buffer size (1KB should be plenty for our responses)
#define MAX_HTTP_RECV_BUFFER 1024

typedef struct
{
    char *buffer;
    int buffer_len;
} response_data_t;

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
    char response_buffer[MAX_HTTP_RECV_BUFFER] = {0};
    response_data_t res = {
        .buffer = response_buffer,
        .buffer_len = 0};

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .event_handler = http_event_handler,
        .user_data = &res,
    };

    esp_http_client_handle_t client = esp_http_client_init(&config);
    if (client == NULL)
    {
        ESP_LOGE(TAG, "Failed to initialize HTTP client");
        return ESP_FAIL;
    }

    // Build JSON request body
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "worker_id", worker_id);
    cJSON_AddStringToObject(root, "worker_type", "esp32");
    cJSON_AddNumberToObject(root, "requested_batch_size", (double)batch_size);

    char *json_str = cJSON_PrintUnformatted(root);

    esp_http_client_set_header(client, "Content-Type", "application/json");
    esp_http_client_set_post_field(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code(client);
        if (status == 200)
        {
            cJSON *resp = cJSON_Parse(response_buffer);
            if (resp)
            {
                out_job->job_id = (int64_t)cJSON_GetObjectItem(resp, "job_id")->valuedouble;
                out_job->nonce_start = (uint32_t)cJSON_GetObjectItem(resp, "nonce_start")->valuedouble;
                out_job->nonce_end = (uint32_t)cJSON_GetObjectItem(resp, "nonce_end")->valuedouble;

                const char *target = cJSON_GetObjectItem(resp, "target_address")->valuestring;
                strncpy(out_job->target_address, target, sizeof(out_job->target_address) - 1);
                out_job->target_address[sizeof(out_job->target_address) - 1] = '\0';

                const char *prefix_b64 = cJSON_GetObjectItem(resp, "prefix_28")->valuestring;
                size_t olen = 0;
                int ret = mbedtls_base64_decode(out_job->prefix_28, sizeof(out_job->prefix_28), &olen,
                                                (const unsigned char *)prefix_b64, strlen(prefix_b64));

                if (ret != 0 || olen != 28)
                {
                    ESP_LOGE(TAG, "Failed to decode prefix_28: %d (len=%d)", ret, (int)olen);
                    err = ESP_FAIL;
                }
                cJSON_Delete(resp);
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
    esp_http_client_cleanup(client);

    return err;
}

esp_err_t api_checkpoint(int64_t job_id, const char *worker_id,
                         uint32_t current_nonce, uint64_t keys_scanned,
                         uint64_t duration_ms)
{
    char url[256];
    snprintf(url, sizeof(url), "%s/api/v1/jobs/%lld/checkpoint", CONFIG_ETHSCANNER_API_URL, job_id);
    ESP_LOGI(TAG, "Sending checkpoint for job %lld to %s", job_id, url);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_PATCH,
    };

    esp_http_client_handle_t client = esp_http_client_init(&config);
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
    // Server expects StartedAt but we don't have it synchronized, it will use zero value

    char *json_str = cJSON_PrintUnformatted(root);

    esp_http_client_set_header(client, "Content-Type", "application/json");
    esp_http_client_set_post_field(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code(client);
        if (status != 200)
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
    if (json_str)
        free(json_str);
    esp_http_client_cleanup(client);

    return err;
}

esp_err_t api_complete(int64_t job_id, const char *worker_id,
                       uint32_t final_nonce, uint64_t keys_scanned,
                       uint64_t duration_ms)
{
    char url[256];
    snprintf(url, sizeof(url), "%s/api/v1/jobs/%lld/complete", CONFIG_ETHSCANNER_API_URL, job_id);
    ESP_LOGI(TAG, "Completing job %lld (final_nonce: %u) (URL: %s)", job_id, (unsigned int)final_nonce, url);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
    };

    esp_http_client_handle_t client = esp_http_client_init(&config);
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

    esp_http_client_set_header(client, "Content-Type", "application/json");
    esp_http_client_set_post_field(client, json_str, strlen(json_str));

    esp_err_t err = esp_http_client_perform(client);

    if (err == ESP_OK)
    {
        int status = esp_http_client_get_status_code(client);
        if (status != 200)
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
    if (json_str)
        free(json_str);
    esp_http_client_cleanup(client);

    return err;
}
