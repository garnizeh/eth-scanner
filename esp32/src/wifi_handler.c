#include <string.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/event_groups.h"
#include "esp_system.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "esp_netif.h"

#include "lwip/err.h"
#include "lwip/sys.h"

#include "wifi_handler.h"

static const char *TAG = "wifi_handler";

/* The event group allows multiple bits for each event, but we only care about two events:
 * - we are connected to the AP with an IP
 * - we failed to connect after the maximum amount of retries */
#define WIFI_CONNECTED_BIT BIT0
#define WIFI_FAIL_BIT BIT1

static EventGroupHandle_t s_wifi_event_group;

static int s_retry_num = 0;
static const int MAX_RETRY = 10;
static const int backoff_delays[] = {1, 2, 5, 10, 30}; // seconds
static const int num_backoff_delays = sizeof(backoff_delays) / sizeof(backoff_delays[0]);

static void event_handler(void *arg, esp_event_base_t event_base,
                          int32_t event_id, void *event_data)
{
    if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_START)
    {
        esp_wifi_connect();
    }
    else if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_DISCONNECTED)
    {
        if (s_retry_num < MAX_RETRY)
        {
            int delay_index = s_retry_num;
            if (delay_index >= num_backoff_delays)
            {
                delay_index = num_backoff_delays - 1;
            }
            int delay_sec = backoff_delays[delay_index];

            ESP_LOGW(TAG, "Disconnected from AP, retrying in %d seconds... (%d/%d)", delay_sec, s_retry_num + 1, MAX_RETRY);

            vTaskDelay(pdMS_TO_TICKS(delay_sec * 1000));
            esp_wifi_connect();
            s_retry_num++;
        }
        else
        {
            ESP_LOGE(TAG, "Max retries reached, restarting...");
            esp_restart();
        }
    }
    else if (event_base == IP_EVENT && event_id == IP_EVENT_STA_GOT_IP)
    {
        ip_event_got_ip_t *event = (ip_event_got_ip_t *)event_data;
        ESP_LOGI(TAG, "Got IP: " IPSTR, IP2STR(&event->ip_info.ip));
        s_retry_num = 0;
        xEventGroupSetBits(s_wifi_event_group, WIFI_CONNECTED_BIT);
    }
}

void wifi_init_sta(void)
{
    s_wifi_event_group = xEventGroupCreate();

    ESP_ERROR_CHECK(esp_netif_init());

    esp_err_t err = esp_event_loop_create_default();
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE)
    {
        ESP_ERROR_CHECK(err);
    }

    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    esp_event_handler_instance_t instance_any_id;
    esp_event_handler_instance_t instance_got_ip;
    ESP_ERROR_CHECK(esp_event_handler_instance_register(WIFI_EVENT,
                                                        ESP_EVENT_ANY_ID,
                                                        &event_handler,
                                                        NULL,
                                                        &instance_any_id));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(IP_EVENT,
                                                        IP_EVENT_STA_GOT_IP,
                                                        &event_handler,
                                                        NULL,
                                                        &instance_got_ip));

    wifi_config_t wifi_config = {
        .sta = {
            .ssid = CONFIG_ETHSCANNER_WIFI_SSID,
            .password = CONFIG_ETHSCANNER_WIFI_PASSWORD,
            .threshold.authmode = WIFI_AUTH_WPA2_PSK,
        },
    };
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_config));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "wifi_init_sta finished, waiting for connection...");

    /* Waiting 100ms to start event. */
    xEventGroupWaitBits(s_wifi_event_group,
                        WIFI_CONNECTED_BIT,
                        pdFALSE,
                        pdFALSE,
                        pdMS_TO_TICKS(5000));

    if (is_wifi_connected())
    {
        ESP_LOGI(TAG, "Connected to SSID:%s", CONFIG_ETHSCANNER_WIFI_SSID);
    }
    else
    {
        ESP_LOGE(TAG, "Failed to connect to WiFi within timeout");
    }
}

bool is_wifi_connected(void)
{
    if (s_wifi_event_group == NULL)
        return false;

    EventBits_t uxBits = xEventGroupGetBits(s_wifi_event_group);
    return (uxBits & WIFI_CONNECTED_BIT) != 0;
}