# Quectel FCS945R 模組配置說明

## 模組資訊

**型號**: Quectel FCS945R  
**晶片組**: Realtek RTL8733BS  
**WiFi 標準**: WiFi 4 (802.11n)  
**頻段**: 2.4GHz only  
**驅動模組**: `8733bs.ko`

---

## XML 配置

### 模組 ID
```xml
<Module id="quectel_fcs945r">
    <DisplayName>Quectel FCS945R (RTL8733BS)</DisplayName>
```

### 核心驅動配置

```xml
<Kernel>
    <KoName>8733bs.ko</KoName>
    <InsmodArgs>rtw_country_code=TW</InsmodArgs>
    <Interface>wlan0</Interface>
    <DriverBackend>nl80211</DriverBackend>
</Kernel>
```

**說明**:
- 驅動模組名稱: `8733bs.ko`
- 加載參數: `rtw_country_code=TW` (台灣區域設定)
- 網路介面: `wlan0`
- 驅動後端: `nl80211` (標準 Linux WiFi API)

**加載驅動指令**:
```bash
insmod 8733bs.ko rtw_country_code=TW
```

---

## Station 模式 (STA)

### XML 配置

```xml
<StationConfig>
    <Param key="ctrl_interface">/var/run/wpa_supplicant</Param>
    <Param key="update_config">1</Param>
</StationConfig>
```

### 生成的 wpa_supplicant.conf

工具會自動生成類似以下內容：

```conf
# wpa_supplicant configuration
# Generated for module: Quectel FCS945R (RTL8733BS)
ctrl_interface=/var/run/wpa_supplicant
update_config=1

# Network configuration
network={
    ssid="ASUS_5566"
    psk="Quectel123"
    scan_ssid=1
}
```

### 手動操作對照

**原始手動指令**:
```bash
wpa_supplicant -Dnl80211 -i wlan0 -c /etc/wpa_supplicant.conf & 
udhcpc -i wlan0
```

**工具自動執行**:
- ✅ 自動偵測 `wpa_supplicant` 路徑
- ✅ 使用 `-Dnl80211 -i wlan0` 參數
- ✅ 生成配置檔案到適當位置
- ✅ 背景啟動 daemon (`-B` 參數)
- ⚠️ **需要手動**: `udhcpc -i wlan0` (DHCP 客戶端)

### 測試流程

1. **選擇模組**: 啟動工具後選擇 `Quectel FCS945R (RTL8733BS)`
2. **選擇 Station 模式**: 選項 2
3. **工具會自動**:
   - 生成 `wpa_supplicant.conf`
   - 啟動 `wpa_supplicant` daemon
   - 掃描可用網路
4. **選擇網路**: 從掃描結果選擇 `ASUS_5566`
5. **輸入密碼**: `Quectel123`
6. **檢查狀態**: 工具會自動執行 `wpa_cli -i wlan0 status`

**額外步驟** (需要手動):
```bash
# 獲取 IP 位址
udhcpc -i wlan0
# 或使用 dhclient
dhclient wlan0
```

---

## AP 模式 (熱點)

### XML 配置

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

### 生成的 hostapd.conf

工具會自動生成類似以下內容：

```conf
# hostapd configuration
# Generated for module: Quectel FCS945R (RTL8733BS)
interface=wlan0
driver=nl80211
ssid=TEST_WIFI
hw_mode=g
channel=6
wpa=2
wpa_passphrase=your_password
wpa_key_mgmt=WPA-PSK
rsn_pairwise=CCMP
ctrl_interface=/var/run/hostapd
ctrl_interface_group=0
beacon_int=100
ieee80211n=1
ht_capab=[HT20][SHORT-GI-20]
```

### 手動操作對照

**原始手動指令**:
```bash
hostapd /etc/hostapd.conf -dd & 
ifconfig wlan0 192.168.11.1 
systemctl stop systemd-resolved 
dnsmasq -i wlan0 -C /etc/dnsmasq.conf &
```

**工具自動執行**:
- ✅ 自動偵測 `hostapd` 路徑
- ✅ 生成 `hostapd.conf` 配置
- ✅ 啟動 `hostapd` daemon
- ⚠️ **不包含**: IP 設定、DHCP server (dnsmasq)

### AP 模式額外配置

**dnsmasq.conf** (需要手動建立):
```conf
user=root
listen-address=192.168.11.1
dhcp-range=192.168.11.2,192.168.11.20
server=/google/8.8.8.8
```

**完整 AP 啟動步驟**:

1. **使用工具啟動 hostapd**:
   ```bash
   ./wifi-tool  # 選擇 FCS945R，選擇 AP 模式
   ```

2. **手動執行額外步驟** (在另一個終端):
   ```bash
   # 設定 IP 位址
   ifconfig wlan0 192.168.11.1 netmask 255.255.255.0
   
   # 停止可能衝突的服務
   systemctl stop systemd-resolved
   
   # 啟動 DHCP server
   dnsmasq -i wlan0 -C /etc/dnsmasq.conf
   ```

---

## 與其他模組的比較

| 特性 | FCS945R | FC21 | FCM360W |
|------|---------|------|---------|
| **晶片組** | RTL8733BS | RTL8821CU | MT7663 |
| **WiFi** | WiFi 4 | WiFi 5 | WiFi 5 |
| **頻段** | 2.4GHz | 2.4/5GHz | 2.4/5GHz |
| **介面** | wlan0 | wlan0 | ra0 |
| **Driver** | nl80211 | nl80211 | nl80211 |
| **特殊參數** | rtw_country_code=TW | rtw_power_mgnt=0 | (無) |

---

## 測試環境

### 預設測試配置

您提供的測試環境已記錄：

```yaml
AP SSID: ASUS_5566
Password: Quectel123
Country: TW (台灣)
Channel: 6 (2.4GHz)
```

工具會在連接時提示您輸入 SSID 和密碼。

### 建議測試流程

#### 測試 Station 模式

```bash
# 1. 加載驅動
insmod /path/to/8733bs.ko rtw_country_code=TW

# 2. 驗證介面
ifconfig wlan0
# 或
ip link show wlan0

# 3. 執行工具
./wifi-tool

# 選擇:
# [3] Quectel FCS945R (RTL8733BS)
# [2] Station 模式
# 選擇網路: ASUS_5566
# 輸入密碼: Quectel123

# 4. 獲取 IP
udhcpc -i wlan0

# 5. 測試連線
ping -c 4 8.8.8.8
```

#### 測試 AP 模式

```bash
# 1. 加載驅動
insmod /path/to/8733bs.ko rtw_country_code=TW

# 2. 執行工具
./wifi-tool

# 選擇:
# [3] Quectel FCS945R (RTL8733BS)
# [3] AP 模式
# 輸入 SSID: TEST_WIFI
# 輸入密碼: test12345
# 選擇頻段: 2.4G

# 3. 在另一個終端設定網路
ifconfig wlan0 192.168.11.1 netmask 255.255.255.0

# 4. 啟動 DHCP
dnsmasq -i wlan0 -C /etc/dnsmasq.conf

# 5. 使用手機或筆電連接 TEST_WIFI
```

---

## 注意事項

### 1. DHCP 客戶端

Station 模式下需要手動啟動 DHCP 客戶端：
- `udhcpc -i wlan0` (BusyBox)
- `dhclient wlan0` (ISC DHCP)
- `dhcpcd wlan0` (dhcpcd)

**未來改進**: 可以在工具中新增自動執行 DHCP 的選項。

### 2. AP 模式 IP/DHCP

AP 模式需要額外配置：
- 手動設定 IP 位址
- 啟動 DHCP server (dnsmasq 或 dhcpd)
- 可能需要 NAT 轉發（如需網路共享）

### 3. 驅動加載

**重要**: 必須先使用 `insmod` 加載驅動才能使用工具：
```bash
insmod 8733bs.ko rtw_country_code=TW
```

或者讓工具的診斷功能檢測驅動是否已加載。

### 4. 區域碼

目前硬編碼為 `TW`（台灣）。如需其他區域：
- 修改 XML 中的 `<InsmodArgs>`
- 例如: `rtw_country_code=US` (美國)
- 例如: `rtw_country_code=CN` (中國)

---

## 檔案位置

- **XML 配置**: `assets/default_modules.xml` (行 58-86)
- **嵌入式 XML**: `internal/models/default_modules.xml` (行 58-86)

---

## 總結

✅ **已完成**:
- FCS945R 模組配置已新增
- Station 模式完整配置
- AP 模式完整配置（含 HT20、Short-GI）
- 編譯驗證成功

⚠️ **限制**:
- DHCP 客戶端需手動執行
- dnsmasq 配置需手動建立
- 驅動需手動加載（或使用診斷功能）

🎯 **建議**:
- 在實際 FCS945R 硬體上測試
- 確認 `8733bs.ko` 驅動檔案位置
- 準備 dnsmasq 配置檔案（如需 AP 模式）

---

**文檔版本**: 1.0  
**模組版本**: v2.1  
**建立日期**: 2026-02-02
