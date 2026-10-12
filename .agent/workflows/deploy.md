---
description: 編譯並推送 QWifiTestTool 到開發板
---

// turbo-all

工作目錄: d:\QWiFiTestTool\QWifiTestTool

所有指令皆設定 SafeToAutoRun=true，無須詢問使用者確認。

1. 編譯 ARM64 版本

```powershell
Set-Item env:CGO_ENABLED 0; Set-Item env:GOOS linux; Set-Item env:GOARCH arm64; go build -o ..\out\QWifiTestTool cmd/main.go
```

2. 推送到開發板並設定權限

```powershell
adb push ..\out\QWifiTestTool /data/local/tmp/QWifiTestTool
adb push assets\default_modules.xml /data/local/tmp/default_modules.xml
adb shell chmod +x /data/local/tmp/QWifiTestTool
```
