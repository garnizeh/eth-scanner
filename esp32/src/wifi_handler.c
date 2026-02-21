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
#include "led_manager.h"

static const char *TAG = "wifi_handler";

#define WIFI_CONNECTED_BIT BIT0
#define WIFI_FAIL_BIT BIT1

static EventGroupHandle_t s_wifi_event_group;
static TimerHandle_t s_wifi_retry_timer = NULL;
static TaskHandle_t s_wifi_bootstrap_task = NULL;
static volatile bool s_wifi_initialized = false;
static volatile bool s_wifi_bootstrap_running = false;

static void retry_timer_callback(TimerHandle_t xTimer);
static void event_handler(void *arg, esp_event_base_t event_base,
                          int32_t event_id, void *event_data);

static int s_retry_num = 0;
static const int MAX_RETRY = 10;
static const int backoff_delays[] = {1, 2, 5, 10, 30}; // seconds
static const int num_backoff_delays = sizeof(backoff_delays) / sizeof(backoff_delays[0]);

static void wifi_bootstrap_task_fn(void *pvParameters)
{
    ESP_LOGI(TAG, "WiFi bootstrap task started.");

    if (s_wifi_event_group == NULL)
    {
        s_wifi_event_group = xEventGroupCreate();
        if (s_wifi_event_group == NULL)
        {
            ESP_LOGE(TAG, "Failed to create WiFi event group.");
            s_wifi_bootstrap_running = false;
            s_wifi_bootstrap_task = NULL;
            vTaskDelete(NULL);
            return;
        }
    }

    if (s_wifi_retry_timer == NULL)
    {
        s_wifi_retry_timer = xTimerCreate("WiFiRetryTimer",
                                          pdMS_TO_TICKS(1000),
                                          pdFALSE,
                                          (void *)0,
                                          retry_timer_callback);
        if (s_wifi_retry_timer == NULL)
        {
            ESP_LOGE(TAG, "Failed to create WiFi retry timer.");
            s_wifi_bootstrap_running = false;
            s_wifi_bootstrap_task = NULL;
            vTaskDelete(NULL);
            return;
        }
    }

    esp_err_t err = esp_netif_init();
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE)
    {
        ESP_LOGE(TAG, "esp_netif_init failed: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    err = esp_event_loop_create_default();
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE)
    {
        ESP_LOGE(TAG, "Failed to create event loop: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    esp_netif_t *sta_netif = esp_netif_create_default_wifi_sta();
    if (sta_netif == NULL)
    {
        ESP_LOGE(TAG, "esp_netif_create_default_wifi_sta failed.");
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    err = esp_wifi_init(&cfg);
    if (err != ESP_OK && err != ESP_ERR_WIFI_INIT_STATE)
    {
        ESP_LOGE(TAG, "esp_wifi_init failed: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    if (err != ESP_ERR_WIFI_INIT_STATE)
    {
        esp_event_handler_instance_t instance_any_id;
        esp_event_handler_instance_t instance_got_ip;

        err = esp_event_handler_instance_register(WIFI_EVENT,
                                                  ESP_EVENT_ANY_ID,
                                                  &event_handler,
                                                  NULL,
                                                  &instance_any_id);
        if (err != ESP_OK)
        {
            ESP_LOGE(TAG, "Register WIFI_EVENT handler failed: %s", esp_err_to_name(err));
            s_wifi_bootstrap_running = false;
            s_wifi_bootstrap_task = NULL;
            vTaskDelete(NULL);
            return;
        }

        err = esp_event_handler_instance_register(IP_EVENT,
                                                  IP_EVENT_STA_GOT_IP,
                                                  &event_handler,
                                                  NULL,
                                                  &instance_got_ip);
        if (err != ESP_OK)
        {
            ESP_LOGE(TAG, "Register IP_EVENT handler failed: %s", esp_err_to_name(err));
            s_wifi_bootstrap_running = false;
            s_wifi_bootstrap_task = NULL;
            vTaskDelete(NULL);
            return;
        }
    }

    wifi_config_t wifi_config = {
        .sta = {
            .ssid = CONFIG_ETHSCANNER_WIFI_SSID,
            .password = CONFIG_ETHSCANNER_WIFI_PASSWORD,
            .threshold.authmode = WIFI_AUTH_WPA2_PSK,
        },
    };

    err = esp_wifi_set_mode(WIFI_MODE_STA);
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "esp_wifi_set_mode failed: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    ESP_LOGI(TAG, "Connecting to SSID: %s", (char *)wifi_config.sta.ssid);
    err = esp_wifi_set_config(WIFI_IF_STA, &wifi_config);
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "esp_wifi_set_config failed: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    ESP_LOGI(TAG, "Starting WiFi... (Min free heap: %lu, bootstrap stack hw: %u)",
             (unsigned long)esp_get_minimum_free_heap_size(),
             (unsigned int)uxTaskGetStackHighWaterMark(NULL));

    err = esp_wifi_start();
    if (err != ESP_OK)
    {
        ESP_LOGE(TAG, "esp_wifi_start failed: %s", esp_err_to_name(err));
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
        vTaskDelete(NULL);
        return;
    }

    ESP_LOGI(TAG, "WiFi driver started. Triggering first connect.");
    err = esp_wifi_connect();
    if (err != ESP_OK)
    {
        ESP_LOGW(TAG, "esp_wifi_connect returned: %s", esp_err_to_name(err));
    }

    s_wifi_initialized = true;
    s_wifi_bootstrap_running = false;
    s_wifi_bootstrap_task = NULL;
    ESP_LOGI(TAG, "WiFi bootstrap complete.");
    vTaskDelete(NULL);
}

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
        ESP_LOGI(TAG, "STA Started.");
        set_led_status(LED_WIFI_CONNECTING);
        // We will call esp_wifi_connect() synchronously after esp_wifi_start()
    }
    else if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_DISCONNECTED)
    {
        if (s_retry_num < MAX_RETRY)
        {
            int delay_sec = backoff_delays[s_retry_num % num_backoff_delays];
            s_retry_num++;

            ESP_LOGW(TAG, "Disconnected. Retry %d/%d scheduled in %d seconds...",
                     s_retry_num, MAX_RETRY, delay_sec);
            set_led_status(LED_WIFI_CONNECTING);

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
            set_led_status(LED_SYSTEM_ERROR);
            xEventGroupSetBits(s_wifi_event_group, WIFI_FAIL_BIT);
        }
    }
    else if (event_base == IP_EVENT && event_id == IP_EVENT_STA_GOT_IP)
    {
        ip_event_got_ip_t *event = (ip_event_got_ip_t *)event_data;
        ESP_LOGI(TAG, "Got IP: " IPSTR, IP2STR(&event->ip_info.ip));
        s_retry_num = 0;
        set_led_status(LED_WIFI_CONNECTED);
        xEventGroupSetBits(s_wifi_event_group, WIFI_CONNECTED_BIT);
    }
}

void wifi_init_sta(void)
{
    ESP_LOGI(TAG, "Entering wifi_init_sta...");

    if (s_wifi_initialized)
    {
        ESP_LOGI(TAG, "WiFi already initialized, requesting reconnect.");
        esp_err_t err = esp_wifi_connect();
        if (err != ESP_OK)
        {
            ESP_LOGW(TAG, "esp_wifi_connect (reconnect) returned: %s", esp_err_to_name(err));
        }
        return;
    }

    if (s_wifi_bootstrap_running)
    {
        ESP_LOGW(TAG, "WiFi bootstrap already running, skipping duplicate init call.");
        return;
    }

    s_wifi_bootstrap_running = true;
    BaseType_t created = xTaskCreatePinnedToCore(
        wifi_bootstrap_task_fn,
        "wifi_bootstrap",
        6144,
        NULL,
        6,
        &s_wifi_bootstrap_task,
        0);

    if (created != pdPASS)
    {
        ESP_LOGE(TAG, "Failed to create WiFi bootstrap task.");
        s_wifi_bootstrap_running = false;
        s_wifi_bootstrap_task = NULL;
    }
}

bool is_wifi_connected(void)
{
    if (s_wifi_event_group == NULL)
        return false;
    EventBits_t uxBits = xEventGroupGetBits(s_wifi_event_group);
    return (uxBits & WIFI_CONNECTED_BIT) != 0;
}