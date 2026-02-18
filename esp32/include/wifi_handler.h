#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#include <stdio.h>

#ifdef __cplusplus
extern "C"
{
#endif

    void wifi_init_sta(void);
    esp_err_t wifi_wait_for_ip(uint32_t timeout_ms);

#ifdef __cplusplus
}
#endif
