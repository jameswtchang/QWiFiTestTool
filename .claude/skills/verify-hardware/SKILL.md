---
name: verify-hardware
description: 透過 ADB 執行 QWifiTestTool 實機端到端驗證 (e2e_wifi_test.ps1，Qualcomm FME170Q)
disable-model-invocation: true
argument-hint: "[-MatrixOnly | -RfOnly | -RfAll | -DiagOnly] [-ApOnly | -StaOnly]"
---

# Verify Hardware：實機硬體閉環端到端驗證

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

透過 `scripts/e2e_wifi_test.ps1` 經 ADB 驅動板端 QWifiTestTool TUI，驗證 AP 配置生成、STA 策略與 AP 實體射頻發射。
腳本預設以 `-ModuleName FME170Q`、`-Iface wlan0` 執行（Qualcomm FME170Q-865 / WCN7851-5）。模組編號依工具選單的名稱自動查找，不依賴 XML 中的順序。

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
| `-MatrixOnly` | 僅 Phase 1 + 2 靜態配置矩陣，不發射射頻 | 約 4 分鐘 |
| `-RfOnly` | 僅核心 6 組射頻實測 | 中 |
| `-RfAll` | 全量 29 組射頻實測（DFS 項目每項最長 120 秒） | 很長 |
| `-ApOnly` / `-StaOnly` | 僅 AP 或僅 STA 項目，可與上列組合 | — |
| `-DiagOnly` | 僅 Phase 4：診斷、驅動卸載／載入／重載、重載後 AP 實測 | 中 |

`-DiagOnly` 會重新載入驅動。執行前必須先徵得使用者同意，並告知：若重載後射頻異常（已知 FME170Q-865 會發生），板子可能需要重開機才能恢復 RF 測試。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 $ARGUMENTS
```

- 參數僅接受上表的 switch 與 `-ModuleName` / `-Iface` / `-CanaryIface` / `-OutDir`；若 `$ARGUMENTS` 含其他內容，停止並向使用者確認。
- 手動在板上執行工具做探測（不經 E2E 腳本）時，存檔動作會覆蓋 `/etc/hostapd.conf` 等檔案且不會自動還原：探測前先自行備份，結束後還原。
- 預設或 `-RfOnly` / `-RfAll` 執行時間較長：以背景方式執行，完成後再讀取輸出。
- 腳本只清理綁定測試網卡的 hostapd / wpa_supplicant / dnsmasq 並重設該網卡，執行期間板端 WiFi 會中斷。
- 板端 `/etc/wpa_supplicant.conf`、`/etc/hostapd.conf`、`/etc/dnsmasq.conf` 會先備份到 `/data/local/tmp/qwt_e2e_backup`，結束時（含 Ctrl+C）自動還原。
- **Exit code**：`0` = 無 FAIL（允許 INCONCLUSIVE），`1` = 有 FAIL 或環境準備失敗。

## 4. 收集異常日誌（有任何測項 FAIL 時）

腳本已自動把失敗項目的證據存到 `-OutDir`（預設 `%TEMP%\qwt_e2e_<時間>`，報告最後一行 `Artifacts:` 會顯示路徑）：

| 檔案 | 內容 |
|:---|:---|
| `results.json` | 所有測項的結果（每次都會產生） |
| `<TC>.tui.log` | 該測項完整的 TUI 畫面紀錄（已去除 ANSI 色碼） |
| `<TC>.hostapd.conf` / `<TC>.wpa_supplicant.conf` | 失敗當下的設定檔 |
| `<TC>.dmesg.log` / `<TC>.iw_info.log` | AP 啟動失敗或射頻斷言失敗時的核心日誌與網卡狀態 |

需要更多資訊時再手動補抓：

```powershell
adb shell "ls -t /tmp/wifi_logs/ 2>/dev/null | head -n 6"
adb shell "cat /tmp/wifi_logs/*.log 2>/dev/null | tail -n 80"
```

## 回報

- 摘錄腳本輸出的 Markdown 報告表與 Summary（Total / Passed / Failed / Inconclusive / Pass Rate）。
- 對每個 FAIL 項目：TC ID、失敗細節、對應 artifact 中的關鍵片段與初步判斷的根因。
- `INCONCLUSIVE` 項目（例如 RF 掃描 0 BSS）設定檔正確但無法實體驗證，不可視為已驗證。
