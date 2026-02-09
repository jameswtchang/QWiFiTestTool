# QWifiTestTool - WiFi 配置與測試工具

跨平台 WiFi 驗證工具，支援 Linux 和 Android (12-16)，專為現場應用工程師設計，用於驗證各種 WiFi 模組（Quectel、Realtek、Qualcomm、MTK 等）。

## 功能特性

✅ **跨平台支援**: Linux (x86/ARM) 和 Android (ARM64, Versions 12-16)  
✅ **零依賴**: 單一靜態二進制檔案  
✅ **XML 配置**: 動態支援多廠商參數  
✅ **三大模式**: 診斷、Station (連接)、AP (熱點)  
✅ **健康檢查**: 介面、驅動、RFKill、核心日誌診斷  

## 目錄結構

```
QWifiTestTool/
├── cmd/
│   └── main.go              # 主程式入口
├── assets/
│   ├── default_modules.xml  # 預設模組配置
│   └── android_arm64/       # Android 靜態二進制檔案 (需自行準備)
│       ├── wpa_supplicant   # v2.10+ (靜態編譯)
│       ├── hostapd          # v2.10+ (靜態編譯)
│       └── iw               # 可選工具
├── internal/
│   ├── models/              # 資料模型 (XML 解析)
│   ├── sys/                 # 系統抽象層 (二進制管理、服務控制)
│   ├── config/              # 配置生成器
│   ├── wifi/                # WiFi 操作邏輯
│   └── ui/                  # CLI 使用者介面
└── go.mod
```

## 快速開始

### 1. 建置

**Linux (x86_64):**

```bash
cd QWifiTestTool
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o wifi-tool cmd/main.go
```

**Android (ARM64):**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o wifi-tool cmd/main.go
```

### 2. 準備 Android 二進制檔案

在 Android 上運行前，需要準備靜態編譯的二進制檔案放入 `assets/android_arm64/`:

- `wpa_supplicant` (v2.10+) - 靜態編譯版本
- `hostapd` (v2.10+) - 靜態編譯版本
- `iw` (可選)

**取得方式**:

1. 從 [wpa_supplicant 原始碼](https://w1.fi/wpa_supplicant/) 編譯
2. 從其他 Android ROM 提取
3. 使用 Buildroot 或 OpenWrt 工具鏈編譯

### 3. 運行

**Linux:**

```bash
sudo ./wifi-tool
```

**Android (需要 Root):**

```bash
adb push wifi-tool /data/local/tmp/
adb shell
su
cd /data/local/tmp
chmod +x wifi-tool
./wifi-tool
```

## 使用方式

### 主選單流程

1. **選擇模組**: 從預設或自訂 XML 中選擇 WiFi 模組
2. **選擇操作**:
   - `[1]` 診斷 - 執行健康檢查
   - `[2]` Station 模式 - 掃描並連接 WiFi
   - `[3]` AP 模式 - 建立熱點

### 診斷功能

檢查項目：

- ✓ 網路介面是否存在
- ✓ 驅動模組是否載入
- ✓ RFKill 硬體封鎖狀態
- ✓ 核心日誌 (dmesg)

### Station 模式 (連接 WiFi)

1. 自動生成 `wpa_supplicant` 配置
2. 啟動 daemon
3. 掃描附近網路
4. 選擇並輸入密碼連接

### AP 模式 (熱點)

1. 輸入熱點名稱 (SSID)
2. 設定密碼 (至少 8 位)
3. 選擇頻段 (2.4G/5G)
4. 啟動 `hostapd`

## 自訂模組配置

### 配置載入優先順序

工具會按以下順序載入配置：

1. **外部配置檔案**（優先）:
   - Linux: `./default_modules.xml` (執行目錄)
   - Android: `/data/local/tmp/default_modules.xml`

2. **內嵌配置**（備援）: 如果外部檔案不存在，使用編譯時內嵌的預設配置

### 使用外部配置

在執行目錄建立或修改 `default_modules.xml`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<WiFiTool>
    <Module id="custom_module">
        <DisplayName>My Custom Module</DisplayName>
        
        <Kernel>
            <KoName>custom.ko</KoName>
            <InsmodArgs>param1=value1</InsmodArgs>
            <Interface>wlan0</Interface>
            <DriverBackend>nl80211</DriverBackend>
        </Kernel>

        <StationConfig>
            <Param key="ctrl_interface">/var/run/wpa_supplicant</Param>
            <Param key="country">US</Param>
        </StationConfig>

        <APConfig>
            <Param key="hw_mode">g</Param>
            <Param key="channel">6</Param>
        </APConfig>
    </Module>
</WiFiTool>
```

### 預設模組

工具內建以下模組：

- Quectel FC21 (RTL8821CU)
- Quectel FCM360W (MT7663)
- Generic Realtek 8188 Series
- Generic Qualcomm Atheros

## Android 支援細節

### 服務接管策略

工具會自動嘗試以下方法禁用 Android WiFi 框架：

1. `cmd wifi set-wifi-enabled disabled` (Android 13+)
2. `svc wifi disable` (Android 12)
3. `pkill -9 wpa_supplicant` (備援)

退出時會自動恢復 WiFi 服務。

### 權限需求

- **Root 權限**: 必須
- **SELinux**: 建議設為 Permissive 模式

## 疑難排解

### 編譯錯誤

**問題**: `embed` 找不到檔案  
**解決**: 確保 `assets/default_modules.xml` 存在

### 運行時錯誤

**問題**: `wpa_supplicant: not found`  
**解決**: 檢查 `assets/android_arm64/` 目錄內容，確保二進制檔案已編譯

**問題**: `failed to disable Android WiFi`  
**解決**: 確認已取得 Root 權限 (`su`)

**問題**: `Interface wlan0 does not exist`  
**解決**:

1. 執行 `insmod {module}.ko` 載入驅動
2. 檢查 XML 中的 `Interface` 名稱是否正確

## 技術細節

### 架構設計

- **Pure Go**: 使用標準庫，無 CGO 依賴
- **嵌入式資源**: `go:embed` 打包配置和二進制檔案
- **平台抽象**: 統一 Linux/Android 差異

### 配置檔案輸出

生成的配置檔案包含時間戳：

- `wpa_supplicant_{module}_{timestamp}.conf`
- `hostapd_{module}_{timestamp}.conf`

儲存位置：

- Linux: `./`
- Android: `/data/local/tmp/wifi_tools/`

## 授權

本專案為內部工具，供現場工程師使用。

## 貢獻

如需新增模組支援，請更新 `assets/default_modules.xml` 並提交 PR。

---

**版本**: 1.0  
**維護**: WiFi 驅動團隊
