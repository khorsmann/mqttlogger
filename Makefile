APP_NAME := mqttlogger
BUILD_DIR := build
MAIN := ./cmd/mqttlogger/main.go
LDFLAGS := -s -w
CGO := 1

all: linux-amd64 linux-arm64 restart

build:
	mkdir -p $(BUILD_DIR)

linux-amd64: build
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO) CC=x86_64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 -ldflags "$(LDFLAGS)" $(MAIN)

linux-arm64: build
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO) CC=aarch64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 -ldflags "$(LDFLAGS)" $(MAIN)

clean:
	rm -rf $(BUILD_DIR)

restart:
	@if systemctl --user status $(APP_NAME).service >/dev/null 2>&1; then \
		systemctl --user restart $(APP_NAME).service; \
	else \
		echo "User-Service $(APP_NAME).service existiert nicht."; \
	fi

help:
	@echo "Makefile targets:"
	@echo "  all          - Build linux-amd64 & linux-arm64 and restart service"
	@echo "  linux-amd64  - Build amd64 binary"
	@echo "  linux-arm64  - Build arm64 binary"
	@echo "  clean        - Remove build directory"
	@echo "  restart      - Restart user service $(APP_NAME)"

.PHONY: all build clean linux-amd64 linux-arm64 restart help

