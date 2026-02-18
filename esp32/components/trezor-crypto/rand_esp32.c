#include "rand.h"
#include "esp_random.h"

void random_reseed(const uint32_t value)
{
    // Not needed for ESP32 hardware TRNG
    (void)value;
}

uint32_t random32(void)
{
    return esp_random();
}
