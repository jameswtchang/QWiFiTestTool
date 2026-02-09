# Quectel FCS945R 模組新增總結

## ✅ 完成事項

已成功將 **Quectel FCS945R (RTL8733BS)** 模組新增到 QWifiTestTool 中。

---

## 模組資訊

| 項目 | 內容 |
|------|------|
| **型號** | Quectel FCS945R |
| **晶片組** | Realtek RTL8733BS |
| **WiFi 標準** | WiFi 4 (802.11n) |
| **頻段** | 2.4GHz only |
| **驅動模組** | `8733bs.ko` |
| **加載參數** | `rtw_country_code=TW` |
| **網路介面** | `wlan0` |
| **驅動後端** | `nl80211` |

---

## XML 配置內容

### 位置
- `assets/default_modules.xml` (行 58-86)
- `internal/models/default_modules.xml` (行 58-86)

### 核心配置

```xml
<Module id="quectel_fcs945r">
    <DisplayName>Quectel FCS945R (RTL8733BS)</DisplayName>
    
    <Kernel>
        <KoName>8733bs.ko</KoName>
        <InsmodArgs>rtw_country_code=TW</InsmodArgs>
        <Interface>wlan0</Interface>
        <DriverBackend>nl80211</DriverBackend>
    </Kernel>
```

### Station 模式

```xml
<StationConfig>
    <Param key="ctrl_interface">/var/run/wpa_supplicant</Param>
    <Param key="update_config">1</Param>
</StationConfig>
```

**對應手動操作**:
```bash
# wpa_supplicant.conf
ctrl_interface=/var/run/wpa_supplicant
update_config=1
network={
    ssid="ASUS_5566"
    psk="Quectel123"
}

# 啟動指令
wpa_supplicant -Dnl80211 -i wlan0 -c /etc/wpa_supplicant.conf &
udhcpc -i wlan0
```

### AP 模式

```xml
<APConfig>
    <Param key="interface">wlan0</Param>
    <Param key="driver">nl80211</Param>
    <Param key="ctrl_interface">/var/run/hostapd</Param>
    <Param key="ctrl_interface_group">0</Param>
    <Param key="beacon_int">100</Param>
    <Param key="hw_mode">g</Param>
    <Param key="channel">6</Param>
    <Param key="ieee80211n">1</Param>
    <Param key="ht_capab">[HT20][SHORT-GI-20]</Param>
</APConfig>
```

**對應手動操作**:
```bash
# hostapd.conf
interface=wlan0
driver=nl80211
ctrl_interface=/var/run/hostapd
ctrl_interface_group=0
beacon_int=100
ssid=TEST_WIFI
channel=6
hw_mode=g
ieee80211n=1
ht_capab=[HT20][SHORT-GI-20]

# 啟動指令
hostapd /etc/hostapd.conf -dd &
ifconfig wlan0 192.168.11.1
dnsmasq -i wlan0 -C /etc/dnsmasq.conf &
```

---

## 工具使用流程

### 啟動工具

```bash
./wifi-tool
```

### 選單顯示

```
========== 可用的 WiFi 模組 ==========
[1] Quectel FC21 (RTL8821CU) (quectel_fc21)
[2] Quectel FCM360W (MT7663) (quectel_fcm360w)
[3] Quectel FCS945R (RTL8733BS) (quectel_fcs945r)  ← 新增
[4] Generic Realtek 8188 Series (generic_rtl8188)
[5] Generic Qualcomm Atheros (generic_qca)
=====================================
選擇模組編號: 3
```

### Station 模式測試

1. 選擇模組 `[3] Quectel FCS945R`
2. 選擇 `[2] Station 模式`
3. 工具會自動掃描網路
4. 選擇 `ASUS_5566`
5. 輸入密碼 `Quectel123`
6. 手動執行: `udhcpc -i wlan0`

### AP 模式測試

1. 選擇模組 `[3] Quectel FCS945R`
2. 選擇 `[3] AP 模式`
3. 輸入 SSID: `TEST_WIFI`
4. 輸入密碼: (您的密碼)
5. 選擇頻段: `2.4G`
6. 手動執行:
   ```bash
   ifconfig wlan0 192.168.11.1
   dnsmasq -i wlan0 -C /etc/dnsmasq.conf
   ```

---

## 編譯結果

```
✅ 編譯成功
檔案: wifi-tool.exe
大小: 3,426,816 bytes (3.43 MB)
時間: 2026-02-02 15:50
狀態: 正常
```

---

## 文檔

已創建詳細配置說明：
**[docs/FCS945R_CONFIG.md](file:///c:/Users/User/.gemini/antigravity/playground/cobalt-helix/QWifiTestTool/docs/FCS945R_CONFIG.md)**

包含內容：
- ✅ 模組完整規格
- ✅ XML 配置詳解
- ✅ Station 模式使用說明
- ✅ AP 模式使用說明
- ✅ 與其他模組比較
- ✅ 測試流程建議
- ✅ 注意事項與限制

---

## 與其他模組比較

| 特性 | **FCS945R** | FC21 | FCM360W |
|------|------------|------|---------|
| 晶片組 | RTL8733BS | RTL8821CU | MT7663 |
| WiFi | WiFi 4 | WiFi 5 | WiFi 5 |
| 頻段 | 2.4GHz | 2.4/5GHz | 2.4/5GHz |
| 介面 | wlan0 | wlan0 | ra0 |
| 驅動 | nl80211 | nl80211 | nl80211 |
| 特殊參數 | rtw_country_code=TW | rtw_power_mgnt=0 | (無) |
| HT 特性 | HT20, Short-GI | 支援 | 支援 |

---

## 重要注意事項

### 1. 驅動加載

**必須先加載驅動**：
```bash
insmod 8733bs.ko rtw_country_code=TW
```

或使用工具的診斷功能檢查驅動狀態。

### 2. DHCP 客戶端 (Station 模式)

工具不包含 DHCP 客戶端，需手動執行：
```bash
udhcpc -i wlan0        # BusyBox
# 或
dhclient wlan0         # ISC DHCP
```

### 3. AP 模式額外配置

需要手動配置：
- IP 位址設定
- DHCP server (dnsmasq)
- 可選：NAT 轉發

**dnsmasq.conf 範例**：
```conf
user=root
listen-address=192.168.11.1
dhcp-range=192.168.11.2,192.168.11.20
server=/google/8.8.8.8
```

### 4. 區域碼

目前設定為 `TW` (台灣)。如需變更：
- 修改 XML 中的 `<InsmodArgs>`
- 支援值: `TW`, `US`, `CN`, `JP` 等

---

## 測試建議

### 準備工作

1. **確認驅動檔案存在**：
   ```bash
   ls -l /path/to/8733bs.ko
   ```

2. **確認測試環境**：
   - AP SSID: `ASUS_5566`
   - Password: `Quectel123`
   - Channel: 6 (2.4GHz)

3. **準備 DHCP 配置**（如需 AP 模式）

### Station 模式測試

```bash
# 1. 加載驅動
insmod /lib/modules/8733bs.ko rtw_country_code=TW

# 2. 驗證介面
ifconfig wlan0

# 3. 執行工具
./wifi-tool
# 選擇: [3] FCS945R → [2] Station → 選網路 → 輸密碼

# 4. 獲取 IP
udhcpc -i wlan0

# 5. 測試
ping -c 4 8.8.8.8
```

### AP 模式測試

```bash
# 1. 加載驅動（同上）

# 2. 執行工具
./wifi-tool
# 選擇: [3] FCS945R → [3] AP → 設定 SSID/密碼

# 3. 設定 IP (另一終端)
ifconfig wlan0 192.168.11.1 netmask 255.255.255.0

# 4. 啟動 DHCP
dnsmasq -i wlan0 -C /etc/dnsmasq.conf

# 5. 客戶端連接測試
```

---

## 檔案變更清單

| 檔案 | 變更 | 說明 |
|------|------|------|
| `assets/default_modules.xml` | 新增 29 行 | FCS945R 模組配置 |
| `internal/models/default_modules.xml` | 新增 29 行 | 嵌入式 XML |
| `docs/FCS945R_CONFIG.md` | 新增 | 詳細說明文檔 |

**總計**: 新增 1 個文檔，更新 2 個 XML 檔案

---

## 總結

✅ **完成項目**:
- FCS945R 模組已新增到 XML 配置
- Station 模式完整支援
- AP 模式完整支援（含 802.11n HT20）
- 驅動參數已配置（Taiwan 區域碼）
- 編譯驗證成功
- 詳細文檔已建立

⚠️ **限制與注意**:
- DHCP 需手動執行（工具不包含）
- dnsmasq 需額外配置（AP 模式）
- 驅動需手動加載或檢查

🎯 **下一步**:
- 在實際 FCS945R 硬體上測試
- 驗證 Taiwan 區域碼是否正確
- 測試 HT20 和 Short-GI 功能

---

**新增版本**: v2.1.1  
**模組數量**: 5 個 (新增 1 個)  
**完成日期**: 2026-02-02  
**測試狀態**: 待實機驗證
