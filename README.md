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

## 目錄結構

```
QWifiTestTool/
├── cmd/
│   └── main.go              # 主程式入口
├── assets/
│   ├── default_modules.xml  # 全域模組配置 (Structured XML)
│   └── android_arm64/       # Android 靜態二進制檔案
├── internal/
│   ├── models/              # 資料模型 (XML 解析、頻道查詢、JSON 嵌入)
│   ├── sys/                 # 系統抽象層
│   ├── config/              # 配置生成器與 TUI 編輯器 (ap_config_editor.go / wpa_editor.go)
│   ├── wifi/                # WiFi 操作邏輯 (Hotspot/Connection)
│   └── ui/                  # 終端使用者介面
├── docs/                    # 專案設計與架構文檔
└── .agent/workflows/        # 自動化工作流 (deploy/push/sync)
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
   - `[2]` 編輯 wpa_supplicant.conf - 修改站台連接配置。
   - `[3]` Station 模式 - 掃描並連接 WiFi。
   - `[4]` Hostapd/Dnsmasq 配置生成及修改 - **(NEW)** 互動式熱點參數配置。
   - `[5]` AP 模式 (建立熱點) - 根據配置啟動 SoftAP。

### 配置編輯器 [4] 

支援從 `/etc/hostapd.conf` 自動載入現有配置，並提供以下互動式修改：

- **⚡ 快速規格預設精靈 (V1.06)**: 一鍵動態過濾模組極限能力，自動完成頻段 (2.4G/5G/6G/Sub-GHz)、協定 (be/ax/ac/n) 與頻寬 (320M~20M) 的組合配置，免除繁瑣計算。
- **國家碼 (TW/CN/US...)**: 自動聯動可選頻道。
- **動態中心頻道**: 自動計算 HT40+/HT40-、VHT 中心頻道，以及 6GHz 的 PSC/Block 基準。
- **即時預覽與儲存**: 即時生成 `hostapd.conf` 與 `dnsmasq.conf` 並儲存至系統路徑。

### STA 模式 [2] 編輯 wpa_supplicant.conf

支援進階測試用連線策略：
- **頻段鎖定 (V1.06)**: 動態查表產生 `freq_list`，支援鎖定 6G、5G(含/排除DFS)、2.4G 頻段。
- **協定上限限制 (V1.06)**: 動態插入 `disable_eht` / `disable_he` / `disable_vht` 以測試向下相容性。

### AP 模式 [5] (優化)

- **自動清理**: 退出 (Ctrl+C) 時自動執行 `killall` 並關閉 `wlan0` 介面。
- **多層級配置**: 優先載入 `/etc/` 配置，其次為 XML 預設值。

## 自訂模組配置 (XML v2)

現在支援更詳細的結構化配置：

```xml
<Module id="quectel_fcs945r">
    <DisplayName>Quectel FCS945R (RTL8733BS)</DisplayName>
    <Kernel>
        <KoName>8733bs.ko</KoName>
        <Interface>wlan0</Interface>
        <DriverBackend>nl80211</DriverBackend>
    </Kernel>

    <APConfig>
        <Capabilities>
            <Bands>2.4G,5G</Bands>
            <Modes>n,ac,ax</Modes>
            <MaxBandwidth>80</MaxBandwidth>
            <SupportDFS>true</SupportDFS>
            <SupportWPA3>true</SupportWPA3>
        </Capabilities>
        <APProfile>
            <SSID>FCS945R_AP</SSID>
            <Security>WPA2</Security>
            <Band>2.4G</Band>
            <Channel>6</Channel>
        </APProfile>
        <Network>
            <GatewayIP>192.168.50.1</GatewayIP>
            <Netmask>255.255.255.0</Netmask>
            <DHCPRange>192.168.50.10,192.168.50.100</DHCPRange>
            <DNSServer>8.8.8.8</DNSServer>
        </Network>
    </APConfig>
</Module>
```

## 支援模組

| # | 模組 | 晶片 | 頻段 | 最高協議 | 狀態 |
|---|------|------|------|----------|------|
| 1 | Quectel FCS945R | RTL8733BS | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |
| 2 | Quectel FCE863R | RTL8852BE | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |
| 3 | Quectel FCS866R | RTL8852BS | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |
| 4 | Quectel FCE867R | RTL8851BU | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |
| 5 | Quectel FCS868R | RTL8852BP | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |
| 6 | Quectel FCE861W | RTL8821CU | 2.4G/5G | WiFi 5 (ac) | ✅ 已驗證 |
| 7 | Quectel FCS945RI | MT7663 | 2.4G/5G | WiFi 5 (ac) | ✅ 已驗證 |
| 8 | Quectel FGH200M | - | Sub-GHz | HaLow (ah) | ✅ 已驗證 |
| 9 | Quectel FME164Q | QCA2064 | 2.4G/5G | WiFi 6 (ax) | ✅ 已驗證 |

## 技術細節

- **頻道資訊庫**: 內嵌 4000+ 行 `country_channel_lookup.json`。
- **Zero-Dependency**: 僅依賴 Go 標準庫，適合嵌入式系統。
- **進程管理**: 使用 Process Group 確保子進程 (hostapd/dnsmasq) 完全終止。
- **CJK 對齊引擎**: 自行實作 East Asian Width 視覺寬度計算，確保中英文標籤在終端機對齊。

---

**版本**: V1.06  
**維護**: WiFi 驅動團隊 (Updated: 2026-09-08)
