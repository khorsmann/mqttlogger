APP_NAME := mqttlogger
BUILD_DIR := build
MAIN := ./cmd/mqttlogger/main.go
LDFLAGS := -s -w
CGO := 1

all: current restart

build:
	mkdir -p $(BUILD_DIR)

current: build
	@echo "Building for current architecture ($(GOOS)/$(GOARCH))..."
	go build -o $(BUILD_DIR)/$(APP_NAME) -ldflags "$(LDFLAGS)" $(MAIN)

linux-amd64: build
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO) CC=x86_64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 -ldflags "$(LDFLAGS)" $(MAIN)

linux-arm64: build
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO) CC=aarch64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 -ldflags "$(LDFLAGS)" $(MAIN)

cross: linux-amd64 linux-arm64

clean:
	go clean -modcache
	go clean -cache
	rm -rf $(BUILD_DIR)

restart:
	@if systemctl --user status $(APP_NAME).service >/dev/null 2>&1; then \
		systemctl --user daemon-reload ; \
		systemctl --user restart $(APP_NAME).service; \
	else \
		echo "User-Service $(APP_NAME).service existiert nicht."; \
	fi

status:
	@if systemctl --user status $(APP_NAME).service >/dev/null 2>&1; then \
		systemctl --user status $(APP_NAME).service; \
	else \
		echo "User-Service $(APP_NAME).service existiert nicht."; \
	fi

logs:
	journalctl --user -u $(APP_NAME).service -f

help:
	@echo "Makefile targets:"
	@echo "  all          - Build current arch & restart service"
	@echo "  clean        - Remove build directory and go cache"
	@echo "  cross        - Build linux-amd64 & linux-arm64 and restart service"
	@echo "  current      - Build current arch & restart service"
	@echo "  help         - Show this help"
	@echo "  linux-amd64  - Build amd64 binary"
	@echo "  linux-arm64  - Build arm64 binary"
	@echo "  logs         - Tail service logs"
	@echo "  restart      - Restart user service $(APP_NAME)"
	@echo "  status       - Show status of the service"


.PHONY: all build current clean linux-amd64 linux-arm64 restart status logs help

