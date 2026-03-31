---
description: 編譯並推送 wifi-tool 到開發板
---

// turbo-all

工作目錄: d:\QWiFiTestTool\QWifiTestTool

所有指令皆設定 SafeToAutoRun=true，無須詢問使用者確認。

1. 同步 XML 配置

```powershell
Copy-Item assets\default_modules.xml internal\models\default_modules.xml -Force
```

1. 編譯 ARM64 版本

```powershell
$env:CGO_ENABLED=0; $env:GOOS='linux'; $env:GOARCH='arm64'; go build -o wifi-tool cmd/main.go
```

1. 推送到開發板並設定權限

```powershell
adb push wifi-tool /data/local/tmp/wifi-tool
adb push assets\default_modules.xml /data/local/tmp/
adb shell chmod +x /data/local/tmp/wifi-tool
```
