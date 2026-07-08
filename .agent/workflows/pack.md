---
description: 打包 QWifiTestTool 執行檔與 XML 配置為 zip
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
$env:CGO_ENABLED=0; $env:GOOS='linux'; $env:GOARCH='arm64'; go build -o QWifiTestTool cmd/main.go
```

1. 打包為 zip

```powershell
Compress-Archive -Path QWifiTestTool, assets\default_modules.xml -DestinationPath QWifiTestTool_V1.01.zip -Force
```
