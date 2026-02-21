#include "led_manager.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

#define LED_PIN 2

static led_status_t current_status = LED_SYSTEM_ERROR;
static SemaphoreHandle_t xActivitySemaphore = NULL;

static void led_task(void *pvParameters)
{
    gpio_reset_pin(LED_PIN);
    gpio_set_direction(LED_PIN, GPIO_MODE_OUTPUT);

    while (1)
    {
        switch (current_status)
        {
        case LED_WIFI_CONNECTING:
            gpio_set_level(LED_PIN, 1);
            vTaskDelay(pdMS_TO_TICKS(100));
            gpio_set_level(LED_PIN, 0);
            vTaskDelay(pdMS_TO_TICKS(100));
            break;

        case LED_WIFI_CONNECTED:
            gpio_set_level(LED_PIN, 1);
            vTaskDelay(pdMS_TO_TICKS(10)); // Ensure we always yield correctly
            gpio_set_level(LED_PIN, 0);
            vTaskDelay(pdMS_TO_TICKS(100)); // Visible blinking for "connected" state
            break;

        case LED_SCANNING:
            if (xSemaphoreTake(xActivitySemaphore, pdMS_TO_TICKS(100)) == pdTRUE)
            {
                gpio_set_level(LED_PIN, 1);
                vTaskDelay(pdMS_TO_TICKS(10));
                gpio_set_level(LED_PIN, 0);
            }
            else
            {
                // Ensure we yield even if no activity
                vTaskDelay(pdMS_TO_TICKS(10));
            }
            break;

        case LED_KEY_FOUND:
            gpio_set_level(LED_PIN, 1);
            vTaskDelay(pdMS_TO_TICKS(50));
            gpio_set_level(LED_PIN, 0);
            vTaskDelay(pdMS_TO_TICKS(50));
            break;

        case LED_SYSTEM_ERROR:
            gpio_set_level(LED_PIN, 1);
            vTaskDelay(pdMS_TO_TICKS(1000));
            gpio_set_level(LED_PIN, 0);
            vTaskDelay(pdMS_TO_TICKS(1000));
            break;

        case LED_OFF:
            gpio_set_level(LED_PIN, 0);
            vTaskDelay(pdMS_TO_TICKS(100)); // Sleep while off
            break;
        }
    }
}

void led_manager_init(void)
{
    xActivitySemaphore = xSemaphoreCreateBinary();
    // Reduce priority to 1 (same as system task) to avoid starving the idle task on Core 0
    xTaskCreatePinnedToCore(led_task, "led_task", 2048, NULL, 1, NULL, 0);
}

void set_led_status(led_status_t status)
{
    current_status = status;
}

void led_trigger_activity(void)
{
    if (xActivitySemaphore != NULL)
    {
        xSemaphoreGive(xActivitySemaphore);
    }
}