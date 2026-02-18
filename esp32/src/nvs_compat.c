#include "nvs_compat.h"
#include "nvs_flash.h"
#include "nvs.h"

// Weak wrapper definitions that call the real NVS functions by default.
// Mark the definitions as weak so the strong stub implementations in
// `test_stubs.c` can provide overrides during unit tests.

esp_err_t __attribute__((weak)) nvs_open_wr(const char *name, nvs_open_mode_t open_mode, nvs_handle_t *out_handle)
{
    return nvs_open(name, open_mode, out_handle);
}

esp_err_t __attribute__((weak)) nvs_get_stats_wr(const char *partition_name, nvs_stats_t *stats)
{
    return nvs_get_stats(partition_name, stats);
}

esp_err_t __attribute__((weak)) nvs_flash_init_wr(void)
{
    return nvs_flash_init();
}

esp_err_t __attribute__((weak)) nvs_flash_erase_wr(void)
{
    return nvs_flash_erase();
}
