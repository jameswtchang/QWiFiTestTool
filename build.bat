@echo off
REM QWifiTestTool Build Script for Windows
REM 
REM Prerequisites:
REM   - Go 1.21+ installed and in PATH
REM

echo ======================================
echo QWifiTestTool Build Script
echo ======================================
echo.

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Go is not installed or not in PATH
    echo Please install Go from: https://go.dev/dl/
    exit /b 1
)

echo [INFO] Go version:
go version
echo.

REM Clean previous builds
if exist QWifiTestTool.exe del /q QWifiTestTool.exe
if exist QWifiTestTool del /q QWifiTestTool

echo.
echo [INFO] Syncing XML configuration files...
echo Source: assets\default_modules.xml
echo Target: internal\models\default_modules.xml
copy /Y assets\default_modules.xml internal\models\default_modules.xml >nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to sync XML files
    exit /b 1
)
echo [SUCCESS] XML files synchronized

echo.
echo [INFO] Running go mod tidy...
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] go mod tidy failed
    exit /b 1
)

echo.
echo [INFO] Building for Windows (testing)...
set CGO_ENABLED=0
go build -o QWifiTestTool.exe cmd/main.go
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Build failed
    exit /b 1
)

echo.
echo [SUCCESS] Build completed: QWifiTestTool.exe
echo.

echo ======================================
echo To build for Linux/Android:
echo ======================================
echo.
echo Linux x86_64:
echo   set CGO_ENABLED=0
echo   set GOOS=linux
echo   set GOARCH=amd64
echo   go build -o QWifiTestTool cmd/main.go
echo.
echo Android ARM64:
echo   set CGO_ENABLED=0
echo   set GOOS=linux
echo   set GOARCH=arm64
echo   go build -o QWifiTestTool cmd/main.go
echo.

pause
