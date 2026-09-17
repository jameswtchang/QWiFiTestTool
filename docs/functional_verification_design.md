# QWiFiTestTool - 全自動實機硬體閉環功能驗證設計規範 (v1.0)

> **版本**: v1.0 (2026-09-15)  
> **測試目標模組**: Quectel FME170Q-865 (Qualcomm WCN7851-5, 支援 WiFi 4~7 及 2.4G/5G/6G)  
> **執行環境**: Host (Windows/Linux via ADB) ➔ Target (ARM64 Linux Kernel 5.10+ nl80211)

---

## 1. 背景與設計原則

在 `protocol_bandwidth_preset_design.md` 中，定義了 AP 規格快速預設精靈（320M/160M/80M/40M/20M 及 Center Channel、Operating Class 計算）與 STA 頻段鎖定/協定上限功能。

為確保這些功能在真實硬體上百分之百正常運作，本驗證規範遵循以下核心原則：

1. **嚴禁「靜態配置檢查替代功能驗證」**：不能只看 `/etc/hostapd.conf` 或 `/etc/wpa_supplicant.conf` 內容是否正確，必須實際進入對應操作模式運行。
2. **AP 模式硬體閉環**：
   - 透過選單 `[4] -> [4]` 選擇預設規格並儲存。
   - 實際進入 **`[5] AP 模式 (建立熱點)`** 啟動服務。
   - **TUI 回顯驗證**：比對 `showAPInfo()` 印出的頻段、模式、頻寬與中心頻道。
   - **核心物理層驗證**：透過 `iw dev wlan0 info` 檢查 `type AP`、頻道頻率與 320M/160M/80M/40M 頻寬是否真實生效，並確認 `hostapd` 行程存活與發射功率 (`txpower != 0.00 dBm`)。
   - **優雅退出**：跨行程發送 `killall -2 QWifiTestTool`（模擬 Ctrl+C），驗證 hostapd 守護進程關閉且網卡狀態復原。
3. **STA 模式硬體閉環**：
   - 透過選單 `[2] -> [1] -> [12/13]` 注入頻段鎖定與協定限制。
   - 實際進入 **`[3] Station 模式 (連接 WiFi)`** 進行關聯連線。
   - **頻率合規斷言**：比對 `連線資訊` 與 `iw dev wlan0 link` 的頻率，確認未連向排除頻段（如 DFS 頻段）。
   - **MCS 調變降級斷言**：檢查 `tx bitrate` 括號中的調變協定標籤（`EHT-MCS` vs `HE-MCS` vs `VHT-MCS` vs `HT-MCS`），確認硬體協商未超出上限。
   - **斷線與清理驗證**：在斷線詢問時輸入 `y`，確認 `iw dev wlan0 link` 回傳 `Not connected.`，且網卡 IP 被徹底 flush 清空。

---

## 2. 測試架構與互動狀態機

```
┌────────────────────────────────────────────────────────┐
│                   Host-Side Test Agent                 │
│              (Python + ADB Controller)                 │
└───────────────────────────┬────────────────────────────┘
                            │  adb shell / PTY
                            ▼
┌────────────────────────────────────────────────────────┐
│               Target SUT: QWifiTestTool                │
│       /data/local/tmp/QWifiTestTool (ARM64 Linux)      │
└──────────────┬──────────────────────────┬──────────────┘
               │                          │
        AP Mode [5]                STA Mode [3]
               │                          │
               ▼                          ▼
   ┌───────────────────────┐  ┌───────────────────────┐
   │        hostapd        │  │    wpa_supplicant     │
   └───────────┬───────────┘  └───────────┬───────────┘
               │                          │
               ▼                          ▼
┌────────────────────────────────────────────────────────┐
│            Linux nl80211 Kernel Driver / RF            │
│       Qualcomm FME170Q (WCN7851 PCIe wlan driver)       │
└────────────────────────────────────────────────────────┘
```

---

## 3. AP 模式測試矩陣 (AP Mode Matrix)

| 測試案例 ID | 測試規格組合 | 精靈選單鍵值 | 預期 TUI 回顯 (`showAPInfo`) | 預期 Linux 實體狀態 (`iw dev info`) | 退出驗證 |
|---|---|---|---|---|---|
| **TC-AP-01** | **6GHz WiFi 7 320M** | `[3] -> [1]` | `6GHz [802.11be WiFi 7 (320MHz)]`<br>`Center: 31, Primary: 37`<br>`WPA3-SAE (CCMP)` | `type AP`<br>`ch 37 (6135 MHz)`<br>`width: 320 MHz, center1: 6105 MHz` | `kill -2` 後<br>`pidof hostapd` 清空 |
| **TC-AP-02** | **6GHz WiFi 6 160M** | `[3] -> [4]` | `6GHz [802.11ax WiFi 6 (160MHz)]`<br>`Center: 47, Primary: 37`<br>`WPA3-SAE (CCMP)` | `type AP`<br>`ch 37 (6135 MHz)`<br>`width: 160 MHz, center1: 6185 MHz` | `kill -2` 後<br>`pidof hostapd` 清空 |
| **TC-AP-03** | **5GHz WiFi 7 160M** | `[2] -> [1]` | `5GHz [802.11be WiFi 7 (160MHz)]`<br>`Center: 50, Primary: 36`<br>`WPA2-PSK (CCMP)` | `type AP`<br>`ch 36 (5180 MHz)`<br>`width: 160 MHz, center1: 5250 MHz` | `kill -2` 後<br>`pidof hostapd` 清空 |
| **TC-AP-04** | **5GHz WiFi 5 80M**  | `[2] -> [7]` | `5GHz [802.11ac WiFi 5]`<br>`Center: 42, Primary: 36`<br>`WPA2-PSK (CCMP)` | `type AP`<br>`ch 36 (5180 MHz)`<br>`width: 80 MHz, center1: 5210 MHz` | `kill -2` 後<br>`pidof hostapd` 清空 |
| **TC-AP-05** | **2.4GHz WiFi 7 40M**| `[1] -> [1]` | `2.4GHz [802.11be WiFi 7]`<br>`channel 1` (HT40+)<br>`WPA2-PSK (CCMP)` | `type AP`<br>`ch 1 (2412 MHz)`<br>`width: 40 MHz` (無 11ac 殘留) | `kill -2` 後<br>`pidof hostapd` 清空 |
| **TC-AP-06** | **2.4GHz WiFi 4 20M**| `[1] -> [6]` | `2.4GHz [802.11n WiFi 4]`<br>`channel 6` (HT20)<br>`WPA2-PSK (CCMP)` | `type AP`<br>`ch 6 (2437 MHz)`<br>`width: 20 MHz` | `kill -2` 後<br>`pidof hostapd` 清空 |

---

## 4. STA 模式測試矩陣 (STA Mode Matrix)

| 測試案例 ID | 測試項目 | 編輯選單鍵值 | 預期連線行為與回顯 (`showConnectionInfo`) | 預期 Linux 實體狀態 (`iw dev link`) | 斷線清潔斷言 |
|---|---|---|---|---|---|
| **TC-STA-01** | **5GHz 頻段鎖定 (排除 DFS)** | `[12] -> [3]` | `頻段/頻道: 5GHz / CH ...`<br>連線頻道嚴禁落在 DFS (CH 52~144) | `freq` 介於 5180~5240 或 5745~5825 MHz<br>無 5260~5720 MHz 頻率 | 輸入 `y` 斷線後<br>`iw link` 為 Not connected<br>IP 完全清空 |
| **TC-STA-02** | **協定限制: 上限 WiFi 6** | `[13] -> [2]` | 連線速率調變類型最高為 `HE-MCS`<br>面對 WiFi 7 AP 嚴禁出現 `EHT-MCS` | `tx bitrate` 包含 `HE-MCS`<br>不含 `EHT-MCS` | 輸入 `y` 斷線後<br>`iw link` 為 Not connected<br>IP 完全清空 |
| **TC-STA-03** | **協定限制: 上限 WiFi 5** | `[13] -> [3]` | 連線速率調變類型最高為 `VHT-MCS`<br>面對 WiFi 6/7 AP 嚴禁出現 `HE/EHT-MCS` | `tx bitrate` 包含 `VHT-MCS`<br>不含 `HE-MCS` 或 `EHT-MCS` | 輸入 `y` 斷線後<br>`iw link` 為 Not connected<br>IP 完全清空 |
| **TC-STA-04** | **協定限制: 上限 WiFi 4** | `[13] -> [4]` | 連線速率調變類型為 `HT-MCS` 或 Legacy (54M)<br>嚴禁出現 `VHT/HE/EHT` | `tx bitrate` 包含 `MCS` 或 `54.0 MBit/s`<br>不含 `VHT/HE/EHT` | 輸入 `y` 斷線後<br>`iw link` 為 Not connected<br>IP 完全清空 |

---

## 5. 測試套件實作規範

測試套件由 `scripts/e2e_wifi_test_agent.py` 驅動，具備以下機制：
1. **防死鎖與非阻塞輸出監聽**：透過背景執行緒即時消費 stdout，若 AP 初始化失敗或連線逾時（設定 30 秒 timeout），自動回收進程並標記 FAIL。
2. **跨程序訊號發送**：AP 啟動並斷言完成後，利用 `adb shell "killall -2 QWifiTestTool"` 觸發內部優雅關閉（執行 `h.Stop()`），並確認 hostapd 完全停止。
3. **自動輸出結構化報告**：測試結束後，自動於終端印出並產出 Markdown 格式總結報告。
