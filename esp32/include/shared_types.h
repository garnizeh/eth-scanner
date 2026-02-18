#ifndef SHARED_TYPES_H
#define SHARED_TYPES_H

#include <stdint.h>
#include <stdbool.h>
#include <stdatomic.h>
#include "nvs.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/timers.h"

// Constants
#define PREFIX_28_SIZE 28
#define ETH_ADDRESS_SIZE 20
#define WORKER_ID_MAX_LEN 32

// Job information structure
typedef struct
{
    int64_t job_id;
    uint8_t prefix_28[PREFIX_28_SIZE];
    uint64_t nonce_start;
    uint64_t nonce_end;
    uint8_t target_address[ETH_ADDRESS_SIZE];
    int64_t expires_at; // Unix timestamp
} job_info_t;

// Checkpoint structure (for NVS persistence)
typedef struct
{
    int64_t job_id;
    uint8_t prefix_28[PREFIX_28_SIZE];
    uint64_t nonce_start;
    uint64_t nonce_end;
    uint64_t current_nonce;
    uint64_t keys_scanned;
    uint64_t timestamp;
    uint32_t magic;
} job_checkpoint_t;

// Worker statistics
typedef struct
{
    uint32_t keys_per_second; // Measured throughput
    uint32_t total_jobs_completed;
    uint64_t total_keys_scanned;
    uint64_t uptime_seconds;
} worker_stats_t;

// Global state structure
typedef struct
{
    // NVS handle
    nvs_handle_t nvs_handle;

    // Current job information
    job_info_t current_job;
    bool job_active;

    // Atomic progress counters (accessed from Core 1 hot loop)
    atomic_ullong current_nonce; // Current nonce being processed
    atomic_ullong keys_scanned;  // Keys scanned in current batch

    // Worker identification
    char worker_id[WORKER_ID_MAX_LEN];

    // Performance metrics
    worker_stats_t stats;

    // Task synchronization
    TaskHandle_t core0_task_handle;
    TaskHandle_t core1_task_handle;

    // Checkpoint timer
    TimerHandle_t checkpoint_timer;

    // State flags
    volatile bool wifi_connected;
    volatile bool should_stop; // Signal worker to stop

} global_state_t;

// Global state instance (defined in main.c)
extern global_state_t g_state;

#endif // SHARED_TYPES_H
