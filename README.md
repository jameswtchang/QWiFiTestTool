# QWifiTestTool - WiFi 配置與測試工具

跨平台 WiFi 驗證工具，支援 Linux 和 Android (12-16)，專為現場應用工程師設計，用於驗證各種 WiFi 模組（Quectel、Realtek、Qualcomm、MTK 等）。

## 功能特性

✅ **跨平台支援**: Linux (x86/ARM) 和 Android (ARM64, Versions 12-16)  
✅ **零依賴**: 單一靜態二進制檔案  
✅ **結構化 XML 配置**: 支援多廠商 Capabilities、APProfile 和 Network 設定  
✅ **四大核心模式**: 診斷、配置編輯器、Station (連接)、AP (熱點)  
✅ **互動式 TUI 編輯器**: 即時修改 SSID、頻段、頻寬、頻道，支援合法頻道驗證  
✅ **國家碼支援**: 內置全球資通訊規範 (Country Code / Channel Lookup)  
✅ **健康檢查**: 介面、驅動、RFKill、核心日誌診斷  
✅ **WiFi 7 & 6GHz 架構**: 完整支援 802.11be、320MHz 頻寬、6GHz 頻道與 PSC 中心頻道動態計算  
✅ **驅動後置命令 (PostLoadCmd)**: 支援 insmod 後自動執行自訂 shell 命令  
✅ **CJK 終端對齊**: 中英文混合標籤在終端機完美視覺對齊  
✅ **STA 頻寬與協定抑制 (V1.11~V1.13)**: 支援 `disable_ht40` (強制 20MHz)、`disable_vht`、`disable_he`、`disable_eht` 及執行期相容性探測  
✅ **DFS CAC 狀態機閉環 (V1.13)**: 5GHz 160M / DFS 頻道主動檢查 `hostapd_cli` (`state=ENABLED`) 與日誌事件，消除假 PASS  
✅ **連線資訊結構化排版 (V1.13)**: STA/AP 模式依「基礎/認證」、「物理射頻」、「網路層」三層分組呈現，清晰專業  
✅ **802.11ah HaLow (Sub-GHz)**: 原生支援 Quectel FGH200M 模組 AP/STA 與 Morse 硬體頻率速率解析  
✅ **雙軌配置生成統一 (V1.14)**: 收斂 `generator.go` 與 `wpa_editor.go`，完整支援 XML `network_` 預設網路與 WPA3-SAE 自動補齊 `ieee80211w=2`  
✅ **即時訊號響應與對稱清理 (V1.14)**: DFS CAC (60s) 等待支援即刻 Ctrl+C 中斷，STA/AP 模式進程指針管理與 `ip route flush dev` 無線路由嚴格隔離  
✅ **驅動參數運行時校驗 (V1.14)**: Sanity Check 擴充 PreloadKo 依賴支援與 `/sys/module/.../parameters` 參數比對，提示即時 reload  
✅ **多層級執行檔解析 (V1.14)**: 四層搜尋策略 (`$PATH` ➔ 自身目錄 ➔ `/usr/sbin` 系統目錄 ➔ fallback)，徹底根除 PATH 盲區  
✅ **XML 單一真理源重構 (V1.14)**: 透過 `package assets` 原生嵌入，徹底根除雙檔案同步陷阱，IDE Run / Debug / go build 即改即生效  
✅ **網卡範圍行程清理 (V1.16)**: STA/AP 啟停、驅動卸載不再全域 `killall`，只清理綁定目標網卡 (或未宣告網卡) 的 wpa_supplicant / hostapd / dnsmasq / DHCP client，eth0 等其他網卡的 DHCP 租約與服務不受影響  
✅ **中文 / 特殊字元 SSID 連線 (V1.16)**: 解碼 `wpa_cli` 的 `\xNN` 跳脫，非 ASCII SSID 以 hex 寫入 `wpa_supplicant.conf`；WPA-PSK 密碼長度與 64 位 hex PSK 格式預先校驗  
✅ **AP 子網路遮罩完整支援 (V1.16)**: XML `<Netmask>` 任意合法遮罩 (如 /25、/22) 正確換算，非法遮罩直接報錯  
✅ **Android 平台修正 (V1.16，尚未實機驗證)**: 內嵌佔位執行檔 ELF 校驗與系統 fallback、無 `iw` 時以 hostapd 狀態判定 AP 就緒、啟動 daemon 前接管 / 結束時還原 Android WiFi framework  
✅ **企業版 802.1X AP 明確標示 (V1.17)**: 掃描清單將純 EAP 網路標為 `WPA2-Enterprise (802.1X)`（原誤標為 Personal），選取時於輸入密碼前即提示不支援；同時支援 EAP 與 PSK 的 AP 仍可用密碼連線  
✅ **英文介面完整翻譯 (V1.17)**: 平台資訊、AP 配置引導、DFS 等待、Morse HaLow 驅動重載等訊息全數納入 i18n，英文模式不再混出中文  
✅ **E2E 測試框架強化 (V1.18)**: 依提示節奏送出輸入並驗證工具正常退出、hostapd.conf 逐行精確比對、`INCONCLUSIVE` 狀態與失敗 exit code、失敗自動留存 TUI 畫面 / 設定檔 / dmesg、板端 `/etc` 設定檔自動備份還原、依名稱選模組  
✅ **E2E 覆蓋擴充 (V1.18)**: 網卡隔離 canary (其他網卡的 daemon 不得被清理誤殺)、英文介面全流程 (AP / STA / RF，畫面不得出現中文)、`-DiagOnly` 診斷與驅動卸載 / 重載測試  
✅ **Station 連線保留與設定持久化 (V1.19)**: 選擇保留連線後離開工具，wpa_supplicant 不再因 SIGPIPE 結束；無系統設定時自動生成的設定每次依 XML 重建但保留上次的網路，自動重連可用；既有連線的中文 SSID 正確顯示  
✅ **E2E 實際連線測試 (V1.19)**: `-StaConnect` / `-StaConnectOnly` 對真實參考 AP 驗證掃描、連線 + DHCP、斷線、自動重連、保留連線與錯誤密碼，密碼於所有輸出遮罩  

## 目錄結構

```
QWifiTestTool/
├── cmd/
│   └── main.go              # 主程式入口
├── assets/
│   ├── default_modules.xml  # 全域模組配置 (唯一真理源 Single Source of Truth)
│   ├── modules.go           # package assets (原生 //go:embed 導出字節)
│   └── android_arm64/       # Android 靜態二進制檔案
├── internal/
│   ├── models/              # 資料模型 (XML 解析、頻道查詢、JSON 嵌入)
│   ├── platform/            # 平台判斷 (IsAndroid，供各套件共用、無其他依賴)
│   ├── sys/                 # 系統抽象層
│   ├── config/              # 配置生成器與 TUI 編輯器 (ap_config_editor.go / wpa_editor.go)
│   ├── wifi/                # WiFi 操作邏輯 (Hotspot/Connection)
│   └── ui/                  # 終端使用者介面
├── docs/                    # 專案設計與架構文檔
├── scripts/                 # E2E 測試套件 (e2e_wifi_test.ps1)、板端清理腳本 (qwt_board_cleanup.sh) 與網卡隔離 canary (qwt_canary.sh)
├── .claude/skills/          # Claude Code 工作流 (/deploy /push /sync /verify-hardware)
└── .agent/workflows/        # 其他 AI IDE 工作流原檔 (保留不更新，現行流程以 .claude/skills 為準)
```

## 快速開始

### 1. 建置與部署

使用內建工作流自動完成同步、編譯與推送到開發板：

```powershell
# 在終端執行
/deploy
```

或手動建置：

```bash
# Android (ARM64)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../out/QWifiTestTool cmd/main.go
adb push ../out/QWifiTestTool /data/local/tmp/
```

### 2. 準備 Android 二進制檔案

在 Android 上運行前，確保 `assets/android_arm64/` 包含以下靜態編譯檔案：

- `wpa_supplicant`
- `hostapd`
- `dnsmasq`

### 3. 運行

```bash
# Android (需要 Root)
adb shell
su
/data/local/tmp/QWifiTestTool
```

## 使用方式

### 主選單流程

1. **選擇模組**: 偵測系統環境並選擇對應的 WiFi 模組專案。
2. **選擇操作**:
   - `[1]` 診斷 (Sanity Check) - 執行環境健康檢查。
   - `[2]` 編輯 wpa_supplicant.conf - 修改站台連接配置（頻段鎖定、協定/頻寬上限）。
   - `[3]` Station 模式 - 掃描並連接 WiFi，顯示三層結構化連線資訊。
   - `[4]` Hostapd/Dnsmasq 配置生成及修改 - 互動式熱點參數配置與快速預設。
   - `[5]` AP 模式 (建立熱點) - 根據配置啟動 SoftAP（含 DFS CAC 狀態機閉環）。

### 配置編輯器 [4] 

支援從 `/etc/hostapd.conf` 自動載入現有配置，並提供以下互動式修改：

- **⚡ 快速規格預設精靈**: 一鍵動態過濾模組極限能力，自動完成頻段 (2.4G/5G/6G/Sub-GHz)、協定 (be/ax/ac/n) 與頻寬 (320M~20M) 的組合配置，免除繁瑣計算。
- **國家碼 (TW/CN/US...)**: 自動聯動可選頻道與法規限制。
- **動態中心頻道與 6GHz PSC**: 自動計算 HT40+/HT40-、VHT 中心頻道，以及 6GHz 的 PSC/Block 基準與 operating class (131-137)。
- **Realtek 5GHz DFS 依賴修正 (V1.13)**: 針對 DFS 頻道強制補齊 `country_code` 與 `ieee80211d=1` 依賴鏈，杜絕 hostapd 啟動退出。
- **即時預覽與儲存**: 即時生成 `hostapd.conf` 與 `dnsmasq.conf` 並儲存至系統路徑，支援唯讀檔案系統自動 remount rw。

### STA 模式 [2] 編輯 wpa_supplicant.conf

支援進階測試用連線策略：
- **頻段鎖定**: 動態查表產生 `freq_list`，支援鎖定 6G、5G(含/排除DFS)、2.4G 頻段，且連線時自動篩選目標 AP。
- **協定上限限制**: 動態插入 `disable_eht` / `disable_he` / `disable_vht` 以測試向下相容性。
- **頻寬上限限制 (V1.11~V1.13)**: 支援插入 `disable_ht40=1` 強制 20MHz 頻寬上限，結合 `wpa_cap` 執行期相容性探測防呆。
- **政策保持**: 掃描連線後自動保留 `freq_list`、`disable_*` 設定，避免重新連線時策略遺失。

### Station 模式連線資訊 [3] (V1.13 優化)

連線成功後以三層結構化排版呈現連線詳情：
- **基礎狀態與認證**: 連線狀態 (COMPLETED)、網卡介面、SSID、BSSID、安全加密 (如 WPA3-SAE / WPA2-PSK)。
- **射頻與鏈路品質**: 頻段/頻道 (含頻寬與中心頻率)、連線速率 (PHY MCS)、接收訊號強度 (RSSI dBm 與品質評級)。
- **網路層**: 配發之 IP 位址 (含子網路遮罩) 與預設閘道 IP。

### AP 模式 [5] (V1.13 優化)

- **DFS CAC 狀態機閉環 (V1.13)**: 啟用 5GHz 160MHz 或 DFS 頻道 (ch52~144) 時，自動執行 60 秒 CAC 監聽，透過 `hostapd_cli status` (`state=ENABLED`) 確保真正發射 Beacon 後才交棒進入就緒狀態。
- **結構化 AP 資訊展示**: 清楚列出介面、驅動、國家碼、SSID、安全性、頻段/模式、實體發射頻道與 TxPower、閘道與 DHCP 範圍。
- **無配置自動引導**: 缺少 AP 配置時直接引導進入 `APConfigEditor` 完成設定並生成配置。
- **自動清理**: 退出 (Ctrl+C) 時自動執行 `killall` 並關閉網卡介面，確保環境乾淨。

## 支援模組

| # | 模組名稱 | 晶片型號 (Chipset) | 頻段 | 最高協定 | 核心驅動 |
|---|----------|-------------------|------|----------|----------|
| 1 | Quectel FCM360W | MediaTek MT7663 | 2.4G/5G | WiFi 5 (802.11ac) | `wlan_mt7663.ko` |
| 2 | Quectel FC21 | Realtek RTL8821CU | 2.4G/5G | WiFi 5 (802.11ac) | `8821cu.ko` |
| 3 | Quectel FCS866R | Realtek RTL8852BS | 2.4G/5G | WiFi 6 (802.11ax) | `8852bs.ko` |
| 4 | Quectel FCE863R | Realtek RTL8852BE | 2.4G/5G | WiFi 6 (802.11ax) | `8852be.ko` |
| 5 | Quectel FCS945R | Realtek RTL8733BS | 2.4G/5G | WiFi 6 (802.11ax) | `8733bs.ko` |
| 6 | Quectel FCU741R | Realtek RTL8733BU | 2.4G/5G | WiFi 6 (802.11ax) | `8733bu.ko` |
| 7 | Quectel FCS961K-L | Aicsemi AIC8800 | 2.4G/5G | WiFi 6 (802.11ax) | `aic8800_fdrv.ko` |
| 8 | Quectel FC905A | Infineon CYW43455 | 2.4G/5G | WiFi 5 (802.11ac) | `bcmdhd.ko` |
| 9 | Quectel FGH200M | Morse Micro MM8108 | Sub-GHz | HaLow (802.11ah) | `morse.ko` (`dot11ah.ko`) |
| 10 | Quectel FME164Q | Qualcomm QCA2064 | 2.4G/5G | WiFi 6 (802.11ax) | `wlan.ko` |
| 11 | Quectel FME170Q-865 | Qualcomm WCN7851-5 | 2.4G/5G/6G | WiFi 7 (802.11be 320M) | `wlan.ko` (`wlan_cnss_core_pcie.ko`) |

## 技術細節

- **頻道資訊庫**: 內嵌 4000+ 行 `country_channel_lookup.json`。
- **Zero-Dependency**: 僅依賴 Go 標準庫，適合嵌入式系統。
- **進程管理**: 直接終止自身啟動的子行程 (hostapd/dnsmasq/wpa_supplicant)，再依 `/proc/<pid>/cmdline` 與設定檔清理綁定目標網卡的殘留 daemon，不影響其他網卡的服務。
- **CJK 對齊引擎**: 自行實作 East Asian Width 視覺寬度計算，確保中英文標籤在終端機對齊。

---

**版本**: V1.19  
**維護**: WiFi 驅動團隊 (Updated: 2026-10-20)

