.PHONY: migrate phone

ifeq ($(OS),Windows_NT)
.PHONY: air

air:
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy/dev.ps1

phone:
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy/connect-phone.ps1 $(if $(DEVICE_SERIAL),-DeviceSerial "$(DEVICE_SERIAL)",)
else
export ADB_PATH DEVICE_SERIAL

phone:
	bash deploy/connect-phone.sh
endif

migrate:
	cd backend && go run ./cmd/migrate -config ./config/config.local.yaml
