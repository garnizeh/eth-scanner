#ifndef MAIN_H
#define MAIN_H

#include "esp_err.h"

// Exposed for unit tests: initialize NVS with erase/retry logic
esp_err_t nvs_init_with_retry(void);

#endif // MAIN_H
