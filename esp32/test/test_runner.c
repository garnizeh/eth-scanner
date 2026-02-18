#include <unity.h>
#include "esp_log.h"
#include "wifi_handler.h"
#include "nvs_flash.h"

extern void test_api_lease_success(void);
extern void test_api_checkpoint(void);
extern void test_api_complete(void);

extern void test_nvs_handler_success(void);
extern void test_nvs_handler_open_error(void);
extern void test_nvs_handler_stats_warning(void);

extern void test_nvs_init_erase_retry(void);

static const char *TAG = "test_runner";

void app_main(void)
{
    ESP_LOGI(TAG, "Starting test runner...");

    // minimal hardware init (stubs may override behavior)
    nvs_flash_init();
    wifi_init_sta();

    UNITY_BEGIN();

    // API client tests
    RUN_TEST(test_api_lease_success);
    RUN_TEST(test_api_checkpoint);
    RUN_TEST(test_api_complete);

    // NVS handler unit tests
    RUN_TEST(test_nvs_handler_success);
    RUN_TEST(test_nvs_handler_open_error);
    RUN_TEST(test_nvs_handler_stats_warning);

    // NVS init retry test
    RUN_TEST(test_nvs_init_erase_retry);

    UNITY_END();
}
