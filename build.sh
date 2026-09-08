#!/bin/bash
# QWifiTestTool Build Script for Linux/macOS
#
# Prerequisites:
#   - Go 1.21+ installed

set -e

echo "======================================"
echo "QWifiTestTool Build Script"
echo "======================================"
echo

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "[ERROR] Go is not installed"
    echo "Please install Go from: https://go.dev/dl/"
    exit 1
fi

echo "[INFO] Go version:"
go version
echo

# Clean previous builds
mkdir -p ../out
rm -f ../out/QWifiTestTool ../out/QWifiTestTool.exe QWifiTestTool QWifiTestTool.exe

echo ""
echo "[INFO] Syncing XML configuration files..."
echo "Source: assets/default_modules.xml"
echo "Target: internal/models/default_modules.xml and ../out/default_modules.xml"
cp assets/default_modules.xml internal/models/default_modules.xml
cp assets/default_modules.xml ../out/default_modules.xml
if [ $? -ne 0 ]; then
    echo "[ERROR] Failed to sync XML files"
    exit 1
fi
echo "[SUCCESS] XML files synchronized"

echo ""
echo "[INFO] Running go mod tidy..."
go mod tidy

echo
echo "[INFO] Select build target:"
echo "[1] Linux x86_64"
echo "[2] Linux ARM64 (Raspberry Pi, etc.)"
echo "[3] Android ARM64"
echo "[4] Current platform"

read -p "Enter choice [1-4]: " choice

case $choice in
    1)
        echo "[INFO] Building for Linux x86_64..."
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../out/QWifiTestTool cmd/main.go
        ;;
    2)
        echo "[INFO] Building for Linux ARM64..."
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../out/QWifiTestTool cmd/main.go
        ;;
    3)
        echo "[INFO] Building for Android ARM64..."
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../out/QWifiTestTool cmd/main.go
        echo
        echo "[INFO] To deploy to Android:"
        echo "  adb push ../out/QWifiTestTool /data/local/tmp/"
        echo "  adb shell chmod +x /data/local/tmp/QWifiTestTool"
        ;;
    4)
        echo "[INFO] Building for current platform..."
        CGO_ENABLED=0 go build -o ../out/QWifiTestTool cmd/main.go
        ;;
    *)
        echo "[ERROR] Invalid choice"
        exit 1
        ;;
esac

if [ -f ../out/QWifiTestTool ]; then
    echo
    echo "[SUCCESS] Build completed: ../out/QWifiTestTool"
    ls -lh ../out/QWifiTestTool
    echo
    echo "[INFO] File type:"
    file ../out/QWifiTestTool
else
    echo "[ERROR] Build failed"
    exit 1
fi

echo
echo "======================================"
echo "Build complete!"
echo "======================================"
