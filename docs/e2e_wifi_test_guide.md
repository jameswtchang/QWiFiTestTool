# E2E WiFi 自動化測試工具使用手冊 (`e2e_wifi_test.ps1`)

完整文檔請參閱 [`scripts/README.md`](file:///d:/QWiFiTestTool/QWifiTestTool/scripts/README.md)。

## 快速速查指令

```powershell
# 1. 全量 29 組 AP 規格實體射頻 (RF) 廣播測試 (100% 硬體發射驗證)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -RfAll

# 2. 靜態全配置矩陣驗證 (Phase 1 AP 30 項 + Phase 2 STA 12 項 + 網卡隔離 1 項，約 4 分鐘)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -MatrixOnly

# 3. 核心 6 組跨頻段射頻快測 (6G/5G/2.4G 快速驗收)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -RfOnly

# 4. 僅測試 STA 頻段鎖定、協定上限與頻寬限制
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -StaOnly

# 5. 標準分層測試 (全矩陣 40 項 + 核心 6 項 RF)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1
```

## 支援參數說明

- **`-RfAll`**: 全量 29 組 AP 規格實體射頻發射測試（包含 6GHz 320M/160M/80M/40M/20M、5GHz 160M/80M/40M/20M、2.4GHz 等全規格）。
- **`-RfOnly`**: 核心 6 組跨頻段射頻實體廣播。
- **`-MatrixOnly`**: 僅驗證 TUI 交互與 `/etc/hostapd.conf`、`/etc/wpa_supplicant.conf` 設定檔生成。
- **`-ApOnly`**: 排除 STA 相關項目，專注 AP 模式。
- **`-StaOnly`**: 排除 AP 相關項目，專注 STA 模式。
- **`-ModuleName <名稱片段>`**（預設 `FME170Q`）: 從工具的模組選單依名稱查找編號，必須恰好匹配一個模組。
- **`-Iface <網卡>`**（預設 `wlan0`）: 測試網卡，清理、掃描與 `iw` 斷言都只針對它。
- **`-OutDir <目錄>`**（預設 `%TEMP%\qwt_e2e_<時間>`）: `results.json` 與失敗證據的輸出目錄。
- **`-CanaryIface <網卡>`**（預設 `eth9`，不需實際存在）: 假 daemon 綁定的網卡，用來驗證清理只針對測試網卡。
- **`-DiagOnly`**: 只跑 Phase 4（診斷選單、驅動卸載／載入／重新載入、重載後 AP 實測）。**需明確指定，預設完整測試不包含**：它會重新載入驅動，若重載後射頻狀態異常，板子可能需要重開機。

## 測試項目數

| 模式 | 項目 |
|:---|:---|
| 預設（完整） | Phase 1 AP 30（29 + 英文 1）+ Phase 2 STA 12（11 + 英文 1）+ Phase 3 RF 7（核心 6 + 英文 1）+ 網卡隔離 1 = **50** |
| `-MatrixOnly` | 30 + 12 + 1 = **43** |
| `-RfOnly` / `-RfAll` | 7 + 1 = **8** / 30 + 1 = **31** |
| `-DiagOnly` | Phase 4 共 4 項 + 網卡隔離 1 = **5** |

## 結果判讀

| 狀態 | 意義 |
|:---|:---|
| `PASS` | 設定檔與（若有）實體 RF 斷言全部通過 |
| `FAIL` | 任一斷言失敗，或輸入與選單脫節、工具未正常結束 |
| `INCONCLUSIVE` | 設定檔正確，但 RF 掃描 0 BSS，無法實體驗證頻段邊界 |

- **Exit code**：`0` = 無 FAIL（允許 INCONCLUSIVE），`1` = 有 FAIL 或環境準備失敗（ADB 未連線、清理腳本推送失敗、找不到模組、備份失敗）。
- **失敗證據**：FAIL 項目自動存下 `<TC>.tui.log`（完整 TUI 畫面紀錄）、當下的設定檔，AP 啟動失敗時另存 `<TC>.dmesg.log`。
- **板端設定檔保護**：`/etc/wpa_supplicant.conf`、`/etc/hostapd.conf`、`/etc/dnsmasq.conf` 開跑前備份到 `/data/local/tmp/qwt_e2e_backup`，結束時（含 Ctrl+C）還原；原本不存在的檔案會被刪除。若上次執行中斷留下備份，會沿用該備份（其中是原始檔）。

## 測試框架強化（A 系列改進）

- **依提示節奏送出輸入**：不再用固定延遲盲送，改為等工具輸出靜止 300ms（代表停在提示上）才送下一筆；每個 session 結束時必須看到「再見!」並正常退出，出現「無效」字樣即判為輸入與選單脫節。
- **AP 矩陣精確比對**：hostapd.conf 逐行比對 `key=value` 完整值（含 `ht_capab` 的 HT20/HT40+、各世代 `*_oper_chwidth` 與中心頻點）；未列入預期的模式與頻寬相關 key 必須不存在（例如 WiFi 6 preset 不得出現 `ieee80211be`）。20M 與 40M preset 互換、WiFi 世代誤標都會被抓到。
- **AP 失敗判斷更新**：改比對 `ui.ShowError` 前綴（`❌ 錯誤:` / `❌ Error:`）與 hostapd 啟動失敗訊息，移除已不存在的舊字串，失敗時不必等到逾時。
- **網卡參數化**：所有 `wlan0` 寫死處改用 `-Iface`。
- **stdin 去 BOM**：.NET 在 `Process.Start` 時會用 `Console.InputEncoding` 建立子行程 stdin，UTF-8 帶 BOM 會讓第一筆輸入變成 `﻿y`，導致語言選擇被忽略。腳本開頭改設為不帶 BOM 的 UTF-8。

## 測試框架強化（B 系列：補齊覆蓋）

- **TC-SYS-01 網卡隔離（`scripts/qwt_canary.sh`）**：開跑前在 `-CanaryIface` 上啟動 udhcpc / dnsmasq / wpa_supplicant / hostapd 四個假 daemon（系統 shell 以 symlink 改名，cmdline 帶 `-i eth9` 或 `interface=eth9` 設定檔）。先做正向對照：清理規則必須把它們認作 eth9 的 daemon，且不能認作測試網卡的；之後每個 Phase 結束檢查一次是否存活，涵蓋工具本身（AP 啟停、驅動卸載）與 `qwt_board_cleanup.sh` 的清理。被誤殺時會列出是在哪個 Phase 之後發現。
- **英文介面（TC-AP-EN01、TC-STA-EN01、TC-RF-EN01）**：語言選 `y` 後跑完整流程，除原本的設定檔／RF 斷言外，語言提示之後的畫面不得出現任何中日韓文字（含全形標點）。
- **Phase 4 診斷與驅動生命週期（`-DiagOnly`）**：以「送出 → 等待預期畫面」逐步驅動。TC-DIAG-01 診斷全通過並出現後續選單；TC-DIAG-02 卸載驅動 → 確認 `/proc/modules` 與網卡消失 → 診斷偵測未載入 → 回答 y 載入 → 確認恢復；TC-DIAG-03 從診斷選單直接重新載入；TC-DIAG-04 重載後實際啟動 AP 並做 RF 斷言。

> **已知問題（V1.17，FME170Q-865）**：工具的「重新載入驅動」只重載 `wlan.ko`（為避免 kernel hang，刻意保留 `wlan_cnss_core_pcie` / `pcie-dw-rockchip`）。重載後 hostapd 可進入 `AP-ENABLED`，但 `iw` 回報 txpower 持續為 `0.00 dBm`，工具對 PCIe 模組的就緒判斷因此逾時，判定 AP 啟動失敗。TC-DIAG-04 會穩定 FAIL。之後模組會卡在異常狀態：`reboot`（暖開機）不會切斷 M.2 插槽電源，重開後 PCIe 仍 `Link Fail`、`wlan_cnss_core_pcie` 回報 `No such device`，**必須完全斷電再開機**才能恢復。

## V1.16 網卡範圍清理

- **板端清理腳本 `scripts/qwt_board_cleanup.sh`**：測試啟動時自動推送到 `/data/local/tmp/`（推送失敗即中止，不退回全域 `killall`）。只清理綁定 `wlan0`（或未宣告網卡）的 wpa_supplicant / hostapd / dnsmasq / DHCP client，eth0 等其他網卡的服務不受影響。需與 `e2e_wifi_test.ps1` 放在同一目錄。
- **RF teardown 斷言範圍收斂**：「hostapd 執行中」與「停止後無殘留」改為只看測試網卡，板上其他網卡的 hostapd 不再造成誤判 FAIL。
- **手動驗證**：`sh /data/local/tmp/qwt_board_cleanup.sh --dry-run wlan0` 只列出將被清理的 PID，不實際終止。

## V1.14 穩健性與架構改進

- **雙軌配置生成統一與 WPA3 自動防護**：收斂 `generator.go` 與 `wpa_editor.go`，全面支援 XML 預設 network（`network_ssid` / `network_sae_password`），並在 SAE 認證下強制注入 `ieee80211w=2` (PMF)。
- **DFS CAC 即時中斷響應**：在 60 秒 CAC 監聽循環中支援 Ctrl+C 即時信號捕獲，可隨時中斷 AP 啟動並閉環清理資源，杜絕 60 秒 hang 阻塞。
- **STA 模式進程追蹤與路由隔離**：新增 `c.wpaCmd` 指針生命週期管理，退出時徹底清除子進程並局部執行 `ip route flush dev <iface>`，隔離無線網卡且不干擾 `eth0`。
- **驅動健全性檢查深度化**：Sanity Check 擴充 PreloadKo 檢查並主動比對 `/sys/module/<mod>/parameters` 參數，參數不一致時提示重新載入。
- **四層執行檔路徑搜尋策略**：消除 Android/Linux PATH 盲區，動態識別 `wpa_supplicant_s1g`、`hostapd_s1g` 與 `dnsmasq`。
- **XML 單一真理源架構**：將 `assets/default_modules.xml` 透過 `assets/modules.go` 原生內嵌，廢除雙檔案複製機制，徹底解決 IDE 直跑與腳本不同步問題。

## V1.13 穩健性與架構改進

- **DFS 頻道可用性檢查 (CAC) 狀態機閉環**：針對 5GHz 160MHz 或 DFS 頻道，深入對接 `hostapd_cli status` (`state=ENABLED`) 及 hostapd 日誌事件 (`DFS-CAC-START`、`DFS-CAC-COMPLETED success=1`、`AP-ENABLED`)，徹底消除 60 秒 CAC 期間提早判定就緒的假 PASS。
- **Realtek 模組 5GHz DFS 依賴修復**：hostapd 規範 `ieee80211h=1` 必須搭配 `ieee80211d=1` 與 `country_code`，針對 Realtek 驅動略過國碼設定的特性在 DFS 頻道強制補齊依賴鏈，防止 daemon 拒絕啟動。
- **STA `disable_ht40` 相容性探測修復**：PowerShell 指令改為單引號傳遞，避免內部雙引號跳脫截斷造成誤送輸入。
- **PowerShell UTF-8 with BOM 編碼**：解決 Windows PowerShell 5.1 在繁中/英文環境下的正則比對失效問題。
- **動態 DFS 超時與錯誤正則精確化**：動態擴展 DFS CAC 測試超時為 120 秒，並精確收窄錯誤字串匹配（防止誤判「Morse 驅動已就緒」）。
- **全量 29 組 AP 實體射頻驗證**：`-RfAll` 模式支援 6G (9組) + 5G (14組) + 2.4G (6組) 完整覆蓋實測。

## V1.10 穩健性改進 (歷史紀錄)

- **iw scan 錯誤偵測**：`iw scan flush` 回傳 `Operation not permitted` / `command failed` / `No such device` / `Device or resource busy` 時，STA 頻段鎖定測試（TC-STA-P01~P05）直接判 FAIL。
- **0-BSS 假陽性防護**：掃描結果為 0 BSS 時標示為 `inconclusive`，不再假 PASS。
- **ReadLineAsync 超時防護**：Phase 3 RF 測試使用非同步讀取 + 1 秒超時，避免 TUI 無輸出時腳本永久 hang。

