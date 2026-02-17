# If running on WSL, ensure `usbipd` is installed and running on the Windows host. Then, list available USB devices and attach the ESP32 device to WSL:
- usbipd list
- usbipd attach --busid 3-3 --wsl