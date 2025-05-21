APP_NAME := mqttlogger
BUILD_DIR := build
GOFILES := main.go
LDFLAGS := -s -w
BUILD_FLAGS := CGO_ENABLED=1

all: linux-amd64 linux-arm64

linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-linux-gnu-gcc go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 -ldflags "$(LDFLAGS)" $(GOFILES)

linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 -ldflags "$(LDFLAGS)" $(GOFILES)

clean:
	rm -rf $(BUILD_DIR)

.PHONY: all clean linux-amd64 linux-arm64

