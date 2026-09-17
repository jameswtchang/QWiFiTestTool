# E2E WiFi 自動化測試工具使用手冊 (`e2e_wifi_test.ps1`)

完整文檔請參閱 [`scripts/README.md`](file:///d:/QWiFiTestTool/QWifiTestTool/scripts/README.md)。

## 快速速查指令

```powershell
# 1. 全量 23 組 AP 規格實體射頻 (RF) 廣播測試 (100% 硬體發射驗證)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -RfAll

# 2. 靜態全配置矩陣驗證 (Phase 1 AP 23 項 + Phase 2 STA 9 項，約 1 分鐘)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -MatrixOnly

# 3. 核心 6 組跨頻段射頻快測 (6G/5G/2.4G 快速驗收)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -RfOnly

# 4. 僅測試 STA 頻段鎖定與協定上限
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 -StaOnly

# 5. 標準分層測試 (全矩陣 32 項 + 核心 6 項 RF)
powershell -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1
```

## 支援參數說明

- **`-RfAll`**: 全量 23 組 AP 規格實體射頻發射測試（包含 6GHz 320M、5GHz 160M/80M/40M/20M、2.4GHz 等全規格）。
- **`-RfOnly`**: 核心 6 組跨頻段射頻實體廣播。
- **`-MatrixOnly`**: 僅驗證 TUI 交互與 `/etc/hostapd.conf`、`/etc/wpa_supplicant.conf` 設定檔生成。
- **`-ApOnly`**: 排除 STA 相關項目，專注 AP 模式。
- **`-StaOnly`**: 排除 AP 相關項目，專注 STA 模式。

## V1.10 穩健性改進

- **iw scan 錯誤偵測**：`iw scan flush` 回傳 `Operation not permitted` / `command failed` / `No such device` / `Device or resource busy` 時，STA 頻段鎖定測試（TC-STA-P01~P05）直接判 FAIL。
- **0-BSS 假陽性防護**：掃描結果為 0 BSS 時標示為 `inconclusive`，不再假 PASS。
- **ReadLineAsync 超時防護**：Phase 3 RF 測試使用非同步讀取 + 1 秒超時，避免 TUI 無輸出時腳本永久 hang。
