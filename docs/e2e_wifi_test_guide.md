# E2E WiFi 自動化測試工具使用手冊 (`e2e_wifi_test.ps1`)

完整文檔請參閱 [`scripts/README.md`](file:///d:/QWiFiTestTool/QWifiTestTool/scripts/README.md)。

## 快速速查指令

```powershell
# 1. 全量 29 組 AP 規格實體射頻 (RF) 廣播測試 (100% 硬體發射驗證)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -RfAll

# 2. 靜態全配置矩陣驗證 (Phase 1 AP 29 項 + Phase 2 STA 11 項，約 1 分鐘)
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

