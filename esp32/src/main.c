#include <stdio.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_system.h"
#include "esp_log.h"

static const char *TAG = "eth-scanner";

void app_main(void)
{
    ESP_LOGI(TAG, "EthScanner ESP32 Worker starting...");

    // Initialization will be added in subsequent tasks

    while (1)
    {
        vTaskDelay(pdMS_TO_TICKS(1000));
    }
}