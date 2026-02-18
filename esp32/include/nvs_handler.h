#ifndef NVS_HANDLER_H
#define NVS_HANDLER_H

#include "esp_err.h"
#include "nvs.h"
#include "shared_types.h"

/**
 * @brief Initialize NVS and open "storage" namespace.
 */
esp_err_t nvs_handler_init(void);

/**
 * @brief Save job checkpoint to NVS atomically using nvs_set_blob.
 */
esp_err_t save_checkpoint(nvs_handle_t handle, const job_checkpoint_t *checkpoint);
/**
 * @brief Load job checkpoint from NVS and validate it.
 */
esp_err_t load_checkpoint(nvs_handle_t handle, job_checkpoint_t *out_checkpoint);
#endif // NVS_HANDLER_H
