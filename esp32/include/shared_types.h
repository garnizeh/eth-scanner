#ifndef SHARED_TYPES_H
#define SHARED_TYPES_H

#include <stdint.h>
#include <stddef.h>

#define PREFIX_28_SIZE 28

/**
 * @brief Job checkpoint structure for atomic persistence in NVS.
 */
typedef struct
{
    int64_t job_id;
    uint8_t prefix_28[PREFIX_28_SIZE];
    uint64_t nonce_start;
    uint64_t nonce_end;
    uint64_t current_nonce;
    uint64_t keys_scanned;
    uint64_t timestamp; // For staleness detection (seconds since boot)
    uint32_t magic;     // 0xDEADBEEF for validity check
} job_checkpoint_t;

#endif // SHARED_TYPES_H
