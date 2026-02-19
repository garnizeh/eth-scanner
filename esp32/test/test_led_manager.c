#include "unity.h"
#include "led_manager.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

void test_led_manager_init(void)
{
    // Ensure that multiple initializations don't crash
    led_manager_init();
    led_manager_init();
}

void test_led_set_status(void)
{
    // Test that we can call all status types without issues
    set_led_status(LED_WIFI_CONNECTING);
    set_led_status(LED_WIFI_CONNECTED);
    set_led_status(LED_SCANNING);
    set_led_status(LED_KEY_FOUND);
    set_led_status(LED_SYSTEM_ERROR);

    // Set to OFF at the end of the test to leave hardware in known state
    set_led_status(LED_OFF);
    vTaskDelay(pdMS_TO_TICKS(150)); // Allow time for task to process OFF
}

void test_led_trigger_activity(void)
{
    // Trigger activity and ensure no crash
    led_trigger_activity();
}
