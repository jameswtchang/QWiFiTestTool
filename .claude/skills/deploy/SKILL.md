---
name: deploy
description: 驗證、編譯 ARM64 版 QWifiTestTool 並推送到開發板 (adb)
disable-model-invocation: true
---

# Deploy：驗證 → 編譯 → 推送到開發板

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

任一步驟失敗即停止，回報錯誤輸出，不要繼續推送。

## 1. 主機端品質檢查

在主機原生平台執行（不可設定 `GOOS`/`GOARCH`，否則測試會編譯成 linux 執行檔而無法在 Windows 執行）：

```powershell
go vet ./...
go test -count=1 ./...
```

## 2. 編譯 ARM64 靜態執行檔

交叉編譯環境變數只在這一次呼叫中生效，結束後立即還原，避免污染同一 session 後續的 `go test`：

```powershell
$env:CGO_ENABLED = '0'; $env:GOOS = 'linux'; $env:GOARCH = 'arm64'
try { go build -o ..\out\QWifiTestTool ./cmd; $rc = $LASTEXITCODE }
finally { Remove-Item env:CGO_ENABLED, env:GOOS, env:GOARCH -ErrorAction SilentlyContinue }
if ($rc -ne 0) { throw "go build failed (exit $rc)" }
Get-Item ..\out\QWifiTestTool | Select-Object Name, Length, LastWriteTime
```

## 3. 確認開發板連線

```powershell
adb devices
```

- 必須恰好一台裝置狀態為 `device`。
- 若無裝置、`unauthorized`、`offline` 或多台裝置，停止並請使用者處理（多台時請使用者指定 serial，再以 `adb -s <serial>` 執行後續指令）。

## 4. 推送執行檔與 XML 配置

```powershell
adb push ..\out\QWifiTestTool /data/local/tmp/QWifiTestTool
adb push assets\default_modules.xml /data/local/tmp/default_modules.xml
adb shell chmod +x /data/local/tmp/QWifiTestTool
```

## 5. 驗證部署結果

```powershell
adb shell ls -l /data/local/tmp/QWifiTestTool /data/local/tmp/default_modules.xml
```

確認板端執行檔大小與步驟 2 的 `Length` 一致、權限含 `x`。

## 回報

簡要列出：測試結果、執行檔大小、推送目標路徑、板端驗證結果。執行方式提示：

```text
adb shell
su
/data/local/tmp/QWifiTestTool
```
