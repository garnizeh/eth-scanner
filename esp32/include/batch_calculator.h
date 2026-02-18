#ifndef BATCH_CALCULATOR_H
#define BATCH_CALCULATOR_H

#include <stdint.h>

/**
 * @brief Calculate the optimal batch size based on throughput and target duration.
 *
 * @param keys_per_second Measured throughput
 * @param target_duration_sec Target lease duration (e.g., 3600 for 1 hour)
 * @return uint32_t Optimized batch size clamped between min/max bounds
 */
uint32_t calculate_batch_size(uint32_t keys_per_second, uint32_t target_duration_sec);

#endif // BATCH_CALCULATOR_H
