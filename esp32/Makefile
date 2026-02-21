# Makefile for ESP32 projects using PlatformIO
# Provides convenient shortcuts for common development tasks

# Upload firmware and start serial monitor
upmon:
	@pio run -t upload -t monitor

# Test with verbose output
test-vv:
	@pio test -e esp32doit-devkit-v1 -vv

# Erase flash memory
erase:
	@pio run -t erase