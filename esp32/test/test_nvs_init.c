#include <unity.h>
#include "main.h" // to get nvs_init_with_retry declaration
#include "nvs_flash.h"

void test_nvs_init_erase_retry(void)
{
    extern int use_nvs_flash_sequence;
    extern esp_err_t nvs_flash_sequence[];
    extern int nvs_flash_sequence_len;
    extern int nvs_flash_init_call_count;
    extern int nvs_erase_count;

    use_nvs_flash_sequence = 1;
    nvs_flash_sequence_len = 2;
    nvs_flash_sequence[0] = ESP_ERR_NVS_NO_FREE_PAGES;
    nvs_flash_sequence[1] = ESP_OK;
    nvs_flash_init_call_count = 0;
    nvs_erase_count = 0;

    esp_err_t ret = nvs_init_with_retry();
    TEST_ASSERT_EQUAL(ESP_OK, ret);
    TEST_ASSERT_EQUAL(1, nvs_erase_count);
    TEST_ASSERT_EQUAL(2, nvs_flash_init_call_count);

    use_nvs_flash_sequence = 0;
}
