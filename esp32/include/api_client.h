#ifndef API_CLIENT_H
#define API_CLIENT_H

#include <stdint.h>
#include "esp_err.h"
#include "shared_types.h"

/**
 * @brief Request a new job lease from the Master API
 *
 * @param worker_id Unique worker identifier
 * @param batch_size Requested number of keys to scan
 * @param out_job Pointer to store the leased job information
 * @return ESP_OK on success, appropriate error code otherwise
 */
esp_err_t api_lease_job(const char *worker_id, uint32_t batch_size,
                        job_info_t *out_job);

/**
 * @brief Update job progress (checkpoint) to the Master API
 *
 * @param job_id ID of the job being processed
 * @param worker_id Unique worker identifier
 * @param current_nonce Current nonce reached in scanning
 * @param keys_scanned Total keys scanned in this session
 * @param duration_ms Time spent scanning in milliseconds
 * @return ESP_OK on success, appropriate error code otherwise
 */
esp_err_t api_checkpoint(int64_t job_id, const char *worker_id,
                         uint64_t current_nonce, uint64_t keys_scanned,
                         uint64_t duration_ms);

/**
 * @brief Mark a job as completed in the Master API
 *
 * @param job_id ID of the job being completed
 * @param worker_id Unique worker identifier
 * @param final_nonce Final nonce reached
 * @param keys_scanned Total keys scanned
 * @param duration_ms Time spent scanning in milliseconds
 * @return ESP_OK on success, appropriate error code otherwise
 */
esp_err_t api_complete(int64_t job_id, const char *worker_id,
                       uint64_t final_nonce, uint64_t keys_scanned,
                       uint64_t duration_ms);

/**
 * @brief Submit a discovered private key to the Master API
 *
 * @param job_id ID of the job that yielded the result
 * @param worker_id Unique worker identifier
 * @param private_key The 32-byte private key found
 * @param address The derived 20-byte address (for verification)
 * @return ESP_OK on success, appropriate error code otherwise
 */
esp_err_t api_submit_result(int64_t job_id, const char *worker_id,
                            const uint8_t *private_key, const uint8_t *address,
                            uint64_t nonce);

#endif // API_CLIENT_H
