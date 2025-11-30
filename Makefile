APP_NAME := mqttlogger
BUILD_DIR := build
MAIN := ./cmd/mqttlogger/main.go
LDFLAGS := -s -w
CGO := 1

INSTALL_DIR := $(HOME)/.local/bin
SERVICE_DIR := $(HOME)/.config/systemd/user
SERVICE_FILE := $(SERVICE_DIR)/$(APP_NAME).service

COLOR_GREEN := \033[32m
COLOR_RED := \033[31m
COLOR_CYAN := \033[36m
COLOR_YELLOW := \033[33m
COLOR_RESET := \033[0m

# ------------------------------------
# Default
# ------------------------------------
all: current restart


# ------------------------------------
# Build targets
# ------------------------------------
build:
	mkdir -p $(BUILD_DIR)

current: build
	@echo -e "$(COLOR_CYAN)Building for current architecture ($(GOOS)/$(GOARCH))...$(COLOR_RESET)"
	go build -o $(BUILD_DIR)/$(APP_NAME) -ldflags "$(LDFLAGS)" $(MAIN)
	@echo -e "$(COLOR_GREEN)✔ Build completed$(COLOR_RESET)"

linux-amd64: build
	@echo -e "$(COLOR_CYAN)Cross-Compiling (linux/amd64)...$(COLOR_RESET)"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO) CC=x86_64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 -ldflags "$(LDFLAGS)" $(MAIN)
	@echo -e "$(COLOR_GREEN)✔ amd64 Binary OK$(COLOR_RESET)"

linux-arm64: build
	@echo -e "$(COLOR_CYAN)Cross-Compiling (linux/arm64)...$(COLOR_RESET)"
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO) CC=aarch64-linux-gnu-gcc \
	go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 -ldflags "$(LDFLAGS)" $(MAIN)
	@echo -e "$(COLOR_GREEN)✔ arm64 Binary OK$(COLOR_RESET)"

cross: linux-amd64 linux-arm64
	@echo -e "$(COLOR_GREEN)✔ Cross Builds complete$(COLOR_RESET)"


# ------------------------------------
# Clean
# ------------------------------------
clean:
	go clean -modcache
	go clean -cache
	rm -rf $(BUILD_DIR)
	@echo -e "$(COLOR_GREEN)✔ Build-Verzeichnis bereinigt.$(COLOR_RESET)"


# ------------------------------------
# Install (binary + systemd service)
# ------------------------------------
install: current
	@echo -e "$(COLOR_CYAN)Installiere Binary nach $(INSTALL_DIR)...$(COLOR_RESET)"
	mkdir -p $(INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@echo -e "$(COLOR_GREEN)✔ Binary installiert.$(COLOR_RESET)"

	@echo -e "$(COLOR_CYAN)Installiere systemd User-Service...$(COLOR_RESET)"
	mkdir -p $(SERVICE_DIR)
	cp ./contrib/$(APP_NAME).service $(SERVICE_FILE)

	systemctl --user daemon-reload
	systemctl --user enable $(APP_NAME).service
	systemctl --user restart $(APP_NAME).service

	@echo -e "$(COLOR_GREEN)✔ Service installiert, aktiviert & gestartet.$(COLOR_RESET)"


# ------------------------------------
# Systemd helpers
# ------------------------------------
restart:
	@if systemctl --user list-unit-files | grep -q "$(APP_NAME).service"; then \
		echo -e "$(COLOR_CYAN)Restarting $(APP_NAME)...$(COLOR_RESET)"; \
		systemctl --user daemon-reload; \
		systemctl --user restart $(APP_NAME).service; \
	else \
		echo -e "$(COLOR_RED)User-Service $(APP_NAME).service existiert nicht.$(COLOR_RESET)"; \
	fi

status:
	@if systemctl --user list-unit-files | grep -q "$(APP_NAME).service"; then \
		systemctl --user status $(APP_NAME).service; \
	else \
		echo -e "$(COLOR_RED)User-Service $(APP_NAME).service existiert nicht.$(COLOR_RESET)"; \
	fi

logs:
	journalctl --user -u $(APP_NAME).service -f


# ------------------------------------
# Help
# ------------------------------------
help:
	@echo -e "$(COLOR_YELLOW)Makefile targets:$(COLOR_RESET)"
	@echo "  all          - Build current arch & restart service"
	@echo "  install      - Build + install binary + install systemd service"
	@echo "  clean        - Remove build directory and go cache"
	@echo "  cross        - Build linux-amd64 & linux-arm64"
	@echo "  current      - Build current arch"
	@echo "  linux-amd64  - Build amd64 binary"
	@echo "  linux-arm64  - Build arm64 binary"
	@echo "  logs         - Tail service logs"
	@echo "  restart      - Restart user service $(APP_NAME)"
	@echo "  status       - Show status of the service"
	@echo "  help         - Show this help"


.PHONY: all build current clean linux-amd64 linux-arm64 cross install restart status logs help
