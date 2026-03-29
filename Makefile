APP_NAME := ccecho
MAIN_PKG := ./cmd/ccecho
DOWNLOAD_DIR := downloads

MACOS_INTEL_DIR := $(DOWNLOAD_DIR)/macos-intel
MACOS_APPLE_DIR := $(DOWNLOAD_DIR)/macos-apple
LINUX_X64_DIR := $(DOWNLOAD_DIR)/linux-x64
LINUX_ARM64_DIR := $(DOWNLOAD_DIR)/linux-arm64
WINDOWS_X64_DIR := $(DOWNLOAD_DIR)/windows-x64
WINDOWS_ARM64_DIR := $(DOWNLOAD_DIR)/windows-arm64

.PHONY: all clean macos-intel macos-apple linux-x64 linux-arm64 windows-x64 windows-arm64

all: macos-intel macos-apple linux-x64 linux-arm64 windows-x64 windows-arm64

clean:
	rm -rf $(DOWNLOAD_DIR)

macos-intel:
	mkdir -p $(MACOS_INTEL_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(MACOS_INTEL_DIR)/$(APP_NAME) $(MAIN_PKG)

macos-apple:
	mkdir -p $(MACOS_APPLE_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(MACOS_APPLE_DIR)/$(APP_NAME) $(MAIN_PKG)

linux-x64:
	mkdir -p $(LINUX_X64_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(LINUX_X64_DIR)/$(APP_NAME) $(MAIN_PKG)

linux-arm64:
	mkdir -p $(LINUX_ARM64_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(LINUX_ARM64_DIR)/$(APP_NAME) $(MAIN_PKG)

windows-x64:
	mkdir -p $(WINDOWS_X64_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(WINDOWS_X64_DIR)/$(APP_NAME).exe $(MAIN_PKG)

windows-arm64:
	mkdir -p $(WINDOWS_ARM64_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $(WINDOWS_ARM64_DIR)/$(APP_NAME).exe $(MAIN_PKG)
