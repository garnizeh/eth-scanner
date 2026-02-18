#include <unity.h>
#include "nvs_handler.h"
#include "shared_types.h"

void test_nvs_handler_success(void)
{
    // default stubs return success
    g_state.nvs_handle = 0;
    esp_err_t err = nvs_handler_init();
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_NOT_EQUAL(0, g_state.nvs_handle);
}

void test_nvs_handler_open_error(void)
{
    extern int stub_nvs_open_behavior;
    stub_nvs_open_behavior = 1;
    g_state.nvs_handle = 0;

    esp_err_t err = nvs_handler_init();
    TEST_ASSERT_NOT_EQUAL(ESP_OK, err);
    TEST_ASSERT_EQUAL(0, g_state.nvs_handle);

    stub_nvs_open_behavior = 0;
}

void test_nvs_handler_stats_warning(void)
{
    extern int stub_nvs_stats_error;
    stub_nvs_stats_error = 1;
    g_state.nvs_handle = 0;

    esp_err_t err = nvs_handler_init();
    TEST_ASSERT_EQUAL(ESP_OK, err);
    TEST_ASSERT_NOT_EQUAL(0, g_state.nvs_handle);

    stub_nvs_stats_error = 0;
}
