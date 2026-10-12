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
if not exist ..\out mkdir ..\out
if exist ..\out\QWifiTestTool.exe del /q ..\out\QWifiTestTool.exe
if exist ..\out\QWifiTestTool del /q ..\out\QWifiTestTool
if exist QWifiTestTool.exe del /q QWifiTestTool.exe
if exist QWifiTestTool del /q QWifiTestTool

echo.
echo [INFO] Preparing external XML configuration for distribution...
echo Source: assets\default_modules.xml
echo Target: ..\out\default_modules.xml
copy /Y assets\default_modules.xml ..\out\default_modules.xml >nul
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to copy XML file to ..\out\
    exit /b 1
)
echo [SUCCESS] XML configuration ready in ..\out\

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
go build -o ..\out\QWifiTestTool.exe cmd/main.go
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Build failed
    exit /b 1
)

echo.
echo [SUCCESS] Build completed: ..\out\QWifiTestTool.exe
echo.

echo ======================================
echo To build for Linux/Android:
echo ======================================
echo.
echo Linux x86_64:
echo   set CGO_ENABLED=0
echo   set GOOS=linux
echo   set GOARCH=amd64
echo   go build -o ..\out\QWifiTestTool cmd/main.go
echo.
echo Android ARM64:
echo   set CGO_ENABLED=0
echo   set GOOS=linux
echo   set GOARCH=arm64
echo   go build -o ..\out\QWifiTestTool cmd/main.go
echo.

pause
