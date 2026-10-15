---
name: verify-hardware
description: 透過 ADB 執行 QWifiTestTool 實機端到端驗證 (e2e_wifi_test.ps1，Qualcomm FME170Q)
disable-model-invocation: true
argument-hint: "[-MatrixOnly | -RfOnly | -RfAll] [-ApOnly | -StaOnly]"
---

# Verify Hardware：實機硬體閉環端到端驗證

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

透過 `scripts/e2e_wifi_test.ps1` 經 ADB 驅動板端 QWifiTestTool TUI，驗證 AP 配置生成、STA 策略與 AP 實體射頻發射。
腳本目前針對 Qualcomm FME170Q-865 (WCN7851-5)、介面 `wlan0` 設計。

本次參數：`$ARGUMENTS`

## 1. 確認開發板與 WiFi 模組狀態

```powershell
adb devices
adb shell "uname -a; lsmod | grep -E 'wlan|cnss'"
```

- 必須恰好一台裝置狀態為 `device`，否則停止並請使用者處理。
- `lsmod` 未見 `wlan` / `cnss` 時，警告使用者驅動可能未載入（射頻項目會失敗），詢問是否繼續。

## 2. 確認板端為最新版本

```powershell
$bin = Get-Item ..\out\QWifiTestTool -ErrorAction SilentlyContinue
Get-ChildItem -Recurse -Include *.go, default_modules.xml cmd, internal, assets |
    Where-Object { -not $bin -or $_.LastWriteTime -gt $bin.LastWriteTime } |
    Select-Object FullName, LastWriteTime
adb shell ls -l /data/local/tmp/QWifiTestTool
```

若有原始碼比編譯產物新、或板端無執行檔：停止，請使用者先執行 `/deploy`。

## 3. 執行 E2E 測試

參數對照（未指定參數時執行完整分層測試）：

| 參數 | 內容 | 預估時間 |
|:---|:---|:---|
| *(無)* | Phase 1 AP 矩陣 29 項 + Phase 2 STA 策略 11 項 + Phase 3 核心射頻 6 項 | 長 |
| `-MatrixOnly` | 僅 Phase 1 + 2 靜態配置矩陣，不發射射頻 | 約 1 分鐘 |
| `-RfOnly` | 僅核心 6 組射頻實測 | 中 |
| `-RfAll` | 全量 29 組射頻實測（DFS 項目每項最長 120 秒） | 很長 |
| `-ApOnly` / `-StaOnly` | 僅 AP 或僅 STA 項目，可與上列組合 | — |

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 $ARGUMENTS
```

- 參數僅接受上表的 switch；若 `$ARGUMENTS` 含其他內容，停止並向使用者確認。
- 預設或 `-RfOnly` / `-RfAll` 執行時間較長：以背景方式執行，完成後再讀取輸出。
- 腳本會以 `killall` 清理板端 hostapd / wpa_supplicant / dnsmasq 並重設 `wlan0`，執行期間板端 WiFi 會中斷。

## 4. 收集異常日誌（有任何測項 FAIL 時）

```powershell
adb shell "dmesg | tail -n 60"
adb shell "ls -t /tmp/wifi_logs/ 2>/dev/null | head -n 6"
adb shell "cat /tmp/wifi_logs/*.log 2>/dev/null | tail -n 80"
```

## 回報

- 摘錄腳本輸出的 Markdown 報告表與 Summary（Total / Passed / Failed / Pass Rate）。
- 對每個 FAIL 項目：TC ID、失敗細節、相關 dmesg / hostapd 日誌片段與初步判斷的根因。
- 標註 `inconclusive`（例如 0 BSS 掃描結果）的項目，不可視為已驗證。
