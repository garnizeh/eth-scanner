#ifndef BENCHMARK_H
#define BENCHMARK_H

#include <stdint.h>

/**
 * @brief Run key generation benchmark using esp_timer_get_time().
 *
 * @return uint32_t throughput in keys/sec
 */
uint32_t benchmark_key_generation(void);

#endif // BENCHMARK_H
