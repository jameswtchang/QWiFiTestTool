---
description: 推送已編譯的 QWifiTestTool 到開發板
---

// turbo-all

工作目錄: d:\QWiFiTestTool\QWifiTestTool

所有指令皆設定 SafeToAutoRun=true，無須詢問使用者確認。

1. 推送 QWifiTestTool 到開發板

```powershell
adb push ..\out\QWifiTestTool /data/local/tmp/
```

1. 推送 XML 配置到開發板

```powershell
adb push assets\default_modules.xml /data/local/tmp/
```

1. 設定執行權限

```powershell
adb shell chmod +x /data/local/tmp/QWifiTestTool
```
