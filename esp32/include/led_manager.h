#ifndef LED_MANAGER_H
#define LED_MANAGER_H

typedef enum
{
    LED_WIFI_CONNECTING,
    LED_WIFI_CONNECTED,
    LED_SCANNING,
    LED_KEY_FOUND,
    LED_SYSTEM_ERROR,
    LED_OFF
} led_status_t;

// Inicializa o hardware e a Task do LED
void led_manager_init(void);

// Muda o estado do LED de qualquer lugar do código
void set_led_status(led_status_t status);

// Notifica a Task do LED que uma atividade de scan ocorreu (Piscada curta)
void led_trigger_activity(void);

#endif