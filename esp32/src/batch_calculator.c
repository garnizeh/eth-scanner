#include <stdint.h>
#include "esp_log.h"
#include "batch_calculator.h"

#define MIN_BATCH_SIZE 10000     // Minimum 10K keys
#define MAX_BATCH_SIZE 10000000  // Maximum 10M keys (ESP32 limit)
#define CHECKPOINT_OVERHEAD 0.05 // 5% reduction for checkpoint time

static const char *TAG = "batch_calc";

uint32_t calculate_batch_size(uint32_t keys_per_second, uint32_t target_duration_sec)
{
    // Handle edge cases
    if (keys_per_second == 0)
    {
        ESP_LOGW(TAG, "Zero throughput, using minimum batch size");
        return MIN_BATCH_SIZE;
    }

    if (target_duration_sec == 0)
    {
        target_duration_sec = 3600; // Default to 1 hour
    }

    // Calculate raw batch size
    uint64_t raw_batch = (uint64_t)keys_per_second * target_duration_sec;

    // Apply checkpoint overhead reduction (~5%)
    raw_batch = (uint64_t)(raw_batch * (1.0 - CHECKPOINT_OVERHEAD));

    // Clamp to min/max bounds
    uint32_t batch_size;
    if (raw_batch < MIN_BATCH_SIZE)
    {
        batch_size = MIN_BATCH_SIZE;
        ESP_LOGW(TAG, "Batch size too small (%llu), clamped to %lu",
                 (unsigned long long)raw_batch, (unsigned long)batch_size);
    }
    else if (raw_batch > MAX_BATCH_SIZE)
    {
        batch_size = MAX_BATCH_SIZE;
        ESP_LOGW(TAG, "Batch size too large (%llu), clamped to %lu",
                 (unsigned long long)raw_batch, (unsigned long)batch_size);
    }
    else
    {
        batch_size = (uint32_t)raw_batch;
    }

    ESP_LOGI(TAG, "Calculated batch size: %lu keys (%.2f hours @ %lu keys/sec)",
             (unsigned long)batch_size,
             (double)batch_size / keys_per_second / 3600.0,
             (unsigned long)keys_per_second);

    return batch_size;
}
