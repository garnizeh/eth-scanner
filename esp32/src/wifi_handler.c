#include <string.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/event_groups.h"
#include "freertos/timers.h"
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

#define WIFI_CONNECTED_BIT BIT0
#define WIFI_FAIL_BIT BIT1

static EventGroupHandle_t s_wifi_event_group;
static TimerHandle_t s_wifi_retry_timer = NULL;

static int s_retry_num = 0;
static const int MAX_RETRY = 10;
static const int backoff_delays[] = {1, 2, 5, 10, 30}; // seconds
static const int num_backoff_delays = sizeof(backoff_delays) / sizeof(backoff_delays[0]);

/**
 * @brief Timer callback that triggers the actual connection attempt.
 * This runs in the Timer Service Task, keeping the Event Loop free.
 */
void retry_timer_callback(TimerHandle_t xTimer)
{
    ESP_LOGI(TAG, "Retry timer expired. Attempting to connect...");
    esp_wifi_connect();
}

static void event_handler(void *arg, esp_event_base_t event_base,
                          int32_t event_id, void *event_data)
{
    if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_START)
    {
        ESP_LOGI(TAG, "STA Started. Connecting...");
        esp_wifi_connect();
    }
    else if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_DISCONNECTED)
    {
        if (s_retry_num < MAX_RETRY)
        {
            int delay_sec = backoff_delays[s_retry_num % num_backoff_delays];
            s_retry_num++;

            ESP_LOGW(TAG, "Disconnected. Retry %d/%d scheduled in %d seconds...",
                     s_retry_num, MAX_RETRY, delay_sec);

            /* Professional approach: Change the period and start the existing timer.
               xTimerChangePeriod also starts the timer if it was idle. */
            if (s_wifi_retry_timer != NULL)
            {
                xTimerChangePeriod(s_wifi_retry_timer, pdMS_TO_TICKS(delay_sec * 1000), 0);
            }
            else
            {
                // Fallback if timer wasn't initialized
                esp_wifi_connect();
            }
        }
        else
        {
            ESP_LOGE(TAG, "Max retries reached.");
            xEventGroupSetBits(s_wifi_event_group, WIFI_FAIL_BIT);
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

void heartbeat_task(void *pvParameters) {
    while(1) {
        ESP_LOGI("HEARTBEAT", "System is still alive...");
        vTaskDelay(pdMS_TO_TICKS(1000));
    }
}

void wifi_init_sta(void)
{
    ESP_LOGI(TAG, "Entering wifi_init_sta...");
    s_wifi_event_group = xEventGroupCreate();

    /* Create the timer once during initialization.
       We set an initial dummy period; it will be changed during retries. */
    s_wifi_retry_timer = xTimerCreate("WiFiRetryTimer",
                                      pdMS_TO_TICKS(1000),
                                      pdFALSE,
                                      (void *)0,
                                      retry_timer_callback);

    ESP_ERROR_CHECK(esp_netif_init());

    esp_err_t err = esp_event_loop_create_default();
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE)
    {
        ESP_LOGE(TAG, "Failed to create event loop: %s", esp_err_to_name(err));
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

    ESP_LOGI(TAG, "Connecting to SSID: %s", (char *)wifi_config.sta.ssid);
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_config));

    xTaskCreate(heartbeat_task, "heartbeat", 2048, NULL, 1, NULL);
    
    ESP_LOGI(TAG, "Starting WiFi...");
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "Waiting for connection bit...");

    /* Note: If you are in a test environment, you might need a longer timeout
       than 5s if your backoff delay is high.
    */
    EventBits_t bits = xEventGroupWaitBits(s_wifi_event_group,
                                           WIFI_CONNECTED_BIT | WIFI_FAIL_BIT,
                                           pdFALSE,
                                           pdFALSE,
                                           pdMS_TO_TICKS(10000)); // Increased to 10s for stability

    if (bits & WIFI_CONNECTED_BIT)
    {
        ESP_LOGI(TAG, "Success! Connected.");
    }
    else
    {
        ESP_LOGE(TAG, "Failed to connect within timeout.");
    }
}

bool is_wifi_connected(void)
{
    if (s_wifi_event_group == NULL)
        return false;
    EventBits_t uxBits = xEventGroupGetBits(s_wifi_event_group);
    return (uxBits & WIFI_CONNECTED_BIT) != 0;
}