#ifndef CORE_TASKS_H
#define CORE_TASKS_H

#include "shared_types.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

extern global_state_t g_state;
extern const char *TAG;

void core0_system_task(void *pvParameters);
void core1_worker_task(void *pvParameters);

/**
 * @brief Spawns Core 0 and initializes the periodic checkpoint timer.
 *        Core 1 is created dynamically only when WiFi is connected.
 */
void start_core_tasks(void);

#endif // CORE_TASKS_H
