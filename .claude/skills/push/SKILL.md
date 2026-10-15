---
name: push
description: 推送已編譯的 QWifiTestTool 與 XML 配置到開發板 (不重新編譯)
disable-model-invocation: true
---

# Push：推送既有編譯產物到開發板

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

只推送 `..\out\QWifiTestTool`，不重新編譯；需要重新編譯請改用 `/deploy`。

## 1. 檢查編譯產物是否過期

```powershell
$bin = Get-Item ..\out\QWifiTestTool -ErrorAction Stop
$newer = Get-ChildItem -Recurse -Include *.go, default_modules.xml cmd, internal, assets |
    Where-Object { $_.LastWriteTime -gt $bin.LastWriteTime }
$bin | Select-Object Name, Length, LastWriteTime
$newer | Select-Object FullName, LastWriteTime
```

- 若執行檔不存在：停止，請使用者改用 `/deploy`。
- 若有原始碼比執行檔新：警告使用者「板端將執行舊版程式」，詢問是否仍要推送或改用 `/deploy`。

## 2. 確認開發板連線

```powershell
adb devices
```

必須恰好一台裝置狀態為 `device`，否則停止並請使用者處理。

## 3. 推送並設定權限

```powershell
adb push ..\out\QWifiTestTool /data/local/tmp/QWifiTestTool
adb push assets\default_modules.xml /data/local/tmp/default_modules.xml
adb shell chmod +x /data/local/tmp/QWifiTestTool
```

## 4. 驗證

```powershell
adb shell ls -l /data/local/tmp/QWifiTestTool /data/local/tmp/default_modules.xml
```

確認板端大小與步驟 1 的 `Length` 一致，回報推送結果。
