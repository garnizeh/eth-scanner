#ifndef GLOBAL_STATE_H
#define GLOBAL_STATE_H

#include "nvs_flash.h"
#include <stdint.h>

typedef struct
{
    nvs_handle_t nvs_handle;
    uint32_t keys_per_second;
} global_state_t;

extern global_state_t g_state;

#endif // GLOBAL_STATE_H
