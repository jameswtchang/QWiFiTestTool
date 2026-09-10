# QWiFiTestTool - AP/STA 快速頻段與協定頻寬配置功能設計文檔 (v4)

> **版本**: v4 (2026-09-07)
> **狀態**: 設計審查修正版（含單位轉換與邊界防呆機制）

## 1. 背景與需求

在 WiFi 測試過程中，頻段（Band）、協定（Protocol: 11be / 11ax / 11ac / 11n / 11ah）與頻寬（Bandwidth: 320M / 160M / 80M / 40M / 20M / 2M）的組合繁多。
- **AP 模式**：手動計算主頻道與中心頻率索引（如 `vht_oper_centr_freq_seg0_idx`、`eht_oper_centr_freq_seg0_idx`）及設定 `op_class` 極易出錯，導致 hostapd 啟動失敗。
- **STA 模式**：Linux 預設由 AP 決定協商規格，缺乏快速「鎖定頻段」或「強制協議降級（如禁用 WiFi 7 測 WiFi 6）」的測試手段。

本功能旨在提供**直覺式的規格預設精靈**，在 AP 與 STA 模式下實現一鍵配置與合規檢查。

---

## 2. 操作模式與交互設計

對齊目前專案的主選單架構：
```text
========== 操作模式 ==========
[1] 診斷 (Sanity Check)
[2] 編輯 wpa_supplicant.conf
[3] Station 模式 (連接 WiFi)
[4] Hostapd/Dnsmasq 配置生成及修改
[5] AP 模式 (建立熱點)
[0] 退出
============================
```

### 2.1 AP 模式：嵌入選單 `[4]` 的子選單

用戶在主選單輸入 `4` 進入 AP 配置主選單。**保持現有選項編號不變**，以新增項目方式加入：

```text
[1] 修改當前配置
[2] 捨棄舊設定，重新生成 (使用 XML 預設值)
[3] 從預設情境範本載入 (快速套用 11n/ac/ax 等情境)
[4] ⚡ 快速規格預設精靈 (頻段/協定/頻寬一鍵配置)  <--- 新增
[0] 退出不修改
```

> **設計原則**：追加而非插入，避免破壞現有用戶的操作記憶。

#### 操作子選單流程：
1. **第一層：選擇頻段**（依據當前模組 `Capabilities` 自動過濾，並標注模組能力上限）：
   ```text
   請選擇頻段:
     [1] 2.4 GHz (最高: be-40M)
     [2] 5 GHz   (最高: be-160M)
     [3] 6 GHz   (最高: be-320M)
     [4] Sub-GHz (802.11ah)              ← 僅 HaLow 模組顯示
     [0] 返回
   ```
   - 若模組不支援某頻段（如無 6G），該選項**不顯示**。
   - 括號內的「最高」標注由 `Capabilities.MaxBandwidth` 和 `Capabilities.Modes` 動態計算。

2. **第二層：選擇協定與頻寬組合**（例如選擇 5 GHz）：
   ```text
   請選擇 5 GHz 規格預設:
     [1] WiFi 7 (802.11be) - 160 MHz  [推薦頻道: 36, Center: 50]
     [2] WiFi 7 (802.11be) - 80 MHz   [推薦頻道: 36, Center: 42]
     [3] WiFi 6 (802.11ax) - 160 MHz  [推薦頻道: 36, Center: 50]
     [4] WiFi 6 (802.11ax) - 80 MHz   [推薦頻道: 36, Center: 42]
     [5] WiFi 6 (802.11ax) - 40 MHz   [推薦頻道: 36]
     [6] WiFi 6 (802.11ax) - 20 MHz   [推薦頻道: 36]
     [7] WiFi 5 (802.11ac) - 80 MHz   [推薦頻道: 36, Center: 42]
     [8] WiFi 5 (802.11ac) - 40 MHz   [推薦頻道: 36]
     [9] WiFi 4 (802.11n)  - 40 MHz   [推薦頻道: 36]
    [10] WiFi 4 (802.11n)  - 20 MHz   [推薦頻道: 36]
     [0] 返回
   ```
   - 超出模組 `MaxBandwidth` 或不支援的 Mode 不顯示。
   - **推薦頻道為國家碼感知**：由 `ChannelLookupDB.LookupChannels(countryCode, apType, band, bandwidth)` 取第一個合法頻道，而非硬編碼。

   > **⚠️ `buildChannelKeys()` 必須修正**：現有 `default` 分支用 `fmt.Sprintf("%s %dM 信道", band, bandwidth)` 產生 JSON key，對 6G 80M 會生成 `"6G 80M 信道"` 但 JSON 中的實際 key 是 `"6G 80M 信道1"`（帶尾碼），導致 6G 80M/160M/40M 查詢回傳空切片。需新增 6G 專屬 case：
   > ```go
   > case band == "6G" && bandwidth == 80:
   >     return []string{"6G 80M 信道1"}
   > case band == "6G" && bandwidth == 160:
   >     return []string{"6G 160M 信道2"}
   > case band == "6G" && bandwidth == 40:
   >     return []string{"6G 40M+ 信道", "6G 40M- 信道"}
   > ```

3. **自動填入並回顯**：
   - 精靈本質是**批次設定 `APEditorParams` 的語法糖**，與手動編輯共用同一份參數結構。
   - 自動設定欄位：`Band`、`Mode`、`Bandwidth`、`Channel`、`Security`（6GHz 強制 WPA3-SAE）。
   - Center Channel 由既有的 `calcVHTCenterChannel()` / `calcHE6GCenterChannel()` 在 `generateHostapd()` 時動態計算，**不需要預先儲存到 params 中**。
   - **保留連線憑證**：精靈僅覆寫 RF/PHY/MAC 相關參數，**必須保留用戶已設定的 SSID、Password 及 CountryCode**。
   - 提示：`✅ 已成功套用 5GHz be-160M 規格 (Channel: 36)`。
   - 用戶可隨後進入 `[1] 手動修改參數` 做微調（例如只改頻道），兩者不衝突。

4. **測試發射**：
   - 用戶存檔返回主選單，輸入 `5` 啟動 AP，即可直接廣播該規格熱點。

---

### 2.2 STA 模式：僅嵌入選單 `[2] 編輯 wpa_supplicant.conf`

> **設計原則**：不在 `[3] Station 模式` 的掃描→連線流程中插入額外步驟，保持快速連線的流暢度。所有策略配置在 `[2]` 中**預先設定好**，連線時自動套用。

用戶在主選單輸入 `[2] 編輯 wpa_supplicant.conf` 後，會先看到當前配置內容與外層選單：

```text
請選擇操作:
[1] 修改當前配置
[2] 捨棄舊設定，重新生成 (使用 XML 預設值)
[0] 退出不修改
```

選擇 `[1] 修改當前配置` 後，在 WPA 編輯器的可編輯參數列表中，新增「連線策略 / 協定限制」區塊：
```text
========== 可編輯參數 ==========
[1]  update_config     (目前: 1)
[2]  ap_scan           (目前: 1)
...
[10] country           (未設定)
[11] ──── 連線頻段與協定限制 ────    <--- 新增分隔
[12] 頻段鎖定           (目前: 自動)
[13] 協定上限限制        (目前: 自動)
[0]  完成編輯
```

#### [12] 頻段鎖定（支援與 [13] 組合使用）：
```text
頻段鎖定:
  [1] 全自動 (不設限)  ← 選此項會主動清除已設定的 freq_list
  [2] 鎖定 6 GHz 頻段
  [3] 鎖定 5 GHz 頻段 (排除 DFS，推薦測試用)
  [4] 鎖定 5 GHz 頻段 (包含 DFS)
  [5] 鎖定 2.4 GHz 頻段
  [0] 返回
```
- **`freq_list` 動態生成與單位轉換**：
  - 根據 `CountryCode` 查詢 `ChannelLookupDB` 取得合法頻道號（Channel Number）。
  - **必須實作 `ChannelToFreq(band, ch)` 將頻道號轉換為頻率 (MHz)**（例如：6GHz 需套用 `5950 + ch*5`），因為 `wpa_supplicant.conf` 僅接受 MHz。

#### [13] 協定上限限制（支援與 [12] 組合使用）：
```text
協定上限限制:
  [1] 全自動 (不設限)  ← 選此項會主動清除 disable_eht/he/vht
  [2] 最高 WiFi 6 (禁用 WiFi 7)   → disable_eht=1
  [3] 最高 WiFi 5 (禁用 WiFi 6/7) → disable_eht=1, disable_he=1
  [4] 最高 WiFi 4 (禁用 WiFi 5/6/7) → disable_eht=1, disable_he=1, disable_vht=1
  [0] 返回
```

#### 組合使用範例：
用戶可以同時設定 `[12] 鎖定 5GHz` + `[13] 最高 WiFi 6`，產出的 `wpa_supplicant.conf` 中 `network={}` 區塊內會同時包含：
```ini
network={
    ssid="TestAP"
    psk="12345678"
    freq_list=5180 5200 5220 5240 5260 5280 ...
    disable_eht=1
}
```

#### 還原機制：
- 選擇 `[12] 全自動` 會移除 `network={}` 中的 `freq_list`。
- 選擇 `[13] 全自動` 會移除 `network={}` 中的 `disable_eht`、`disable_he`、`disable_vht`。
- 確保測試完畢後可一鍵恢復正常自動協商。

---

## 3. 設定檔變更對照 (Configuration Mapping)

### 3.1 `hostapd.conf` 變更（AP 模式）

以下數值已對齊 `ap_config_editor.go` 中 `generateHostapd()` 的實際產生邏輯。

> **hostapd chwidth 定義**：
> - `eht_oper_chwidth`: 0=20MHz, 1=80MHz, 2=160MHz, 3=320MHz
> - `he_oper_chwidth`: 0=HT20/40, 1=80MHz, 2=160MHz
> - `vht_oper_chwidth`: 0=20/40MHz, 1=80MHz, 2=160MHz

#### 6 GHz 頻段

| 組合標籤 | 核心參數產出 |
| :--- | :--- |
| **6G be-320M** | `hw_mode=a`, `ieee80211be=1`, `ieee80211ax=1`, `eht_oper_chwidth=3`, `he_oper_chwidth=2`, `op_class=137`, Center 由 `calcHE6GCenterChannel()` 動態計算, `wpa_key_mgmt=SAE`, `ieee80211w=2` |
| **6G be-160M** | `hw_mode=a`, `ieee80211be=1`, `ieee80211ax=1`, `eht_oper_chwidth=2`, `he_oper_chwidth=2`, `op_class=134`, `wpa_key_mgmt=SAE`, `ieee80211w=2` |
| **6G be-80M**  | `hw_mode=a`, `ieee80211be=1`, `ieee80211ax=1`, `eht_oper_chwidth=1`, `he_oper_chwidth=1`, `op_class=133`, `wpa_key_mgmt=SAE`, `ieee80211w=2` |
| **6G ax-160M** | `hw_mode=a`, `ieee80211ax=1`, `he_oper_chwidth=2`, `op_class=134`, `wpa_key_mgmt=SAE`, `ieee80211w=2` |
| **6G ax-80M**  | `hw_mode=a`, `ieee80211ax=1`, `he_oper_chwidth=1`, `op_class=133`, `wpa_key_mgmt=SAE`, `ieee80211w=2` |

> **6GHz Center Channel 參考值**（ch37 為例）：
> - 6GHz AP 規範要求主頻道必須使用 PSC (如 37, 53, 69 等)，否則 STA 難以掃描到。
> - Center Channel 必須由 `calcHE6GCenterChannel(channel)` 嚴格計算對齊標準 Block：
>   - 320M → 若主頻道 37 (落於 Block 1)，則 center=31（31±30 範圍）
>   - 160M → center=47（47±14 範圍）
>   - 80M  → center=39（39±6 範圍）

> **⚠️ 6GHz `op_class` 完整對照**：
> - 320 MHz → `op_class=137`
> - 160 MHz → `op_class=134`
> - 80 MHz  → `op_class=133`
> - 40 MHz  → `op_class=132`（現有程式碼缺少此分支，直接跳到 131）
> - 20 MHz  → `op_class=131`
>
> 現有 `generateHostapd()` 的 else 分支將 40M 歸入 20M 的 `op_class=131`，需修正為 `bandwidth == 40 → op_class=132`。

#### 5 GHz 頻段

| 組合標籤 | 核心參數產出 |
| :--- | :--- |
| **5G be-160M** | `hw_mode=a`, `ieee80211be=1`, `ieee80211ax=1`, `ieee80211ac=1`, `ieee80211n=1`, `eht_oper_chwidth=2`, `he_oper_chwidth=2`, `vht_oper_chwidth=2`, `ht_capab=[HT40+]...` |
| **5G be-80M**  | 同上但 `eht_oper_chwidth=1`, `he_oper_chwidth=1`, `vht_oper_chwidth=1` |
| **5G ax-160M** | `hw_mode=a`, `ieee80211ax=1`, `ieee80211ac=1`, `ieee80211n=1`, `he_oper_chwidth=2`, `vht_oper_chwidth=2`, `ht_capab=[HT40+]...` |
| **5G ax-80M**  | 同上但 `he_oper_chwidth=1`, `vht_oper_chwidth=1` |
| **5G ax-40M**  | `hw_mode=a`, `ieee80211ax=1`, `ieee80211ac=1`, `ieee80211n=1`, `ht_capab=[HT40+/HT40-]` |
| **5G ax-20M**  | `hw_mode=a`, `ieee80211ax=1`, `ieee80211ac=1`, `ieee80211n=1`, `ht_capab=[HT20]` |
| **5G ac-80M**  | `hw_mode=a`, `ieee80211ac=1`, `ieee80211n=1`, `vht_oper_chwidth=1`, `ht_capab=[HT40+]...` |
| **5G ac-40M**  | `hw_mode=a`, `ieee80211ac=1`, `ieee80211n=1`, `ht_capab=[HT40+/HT40-]` |
| **5G n-40M**   | `hw_mode=a`, `ieee80211n=1`, `ht_capab=[HT40+/HT40-]` |
| **5G n-20M**   | `hw_mode=a`, `ieee80211n=1`, `ht_capab=[HT20]` |

> **5GHz 注意事項**：
> - `op_class` 不由程式顯式寫入（hostapd 從 hw_mode + channel + bandwidth 自動推斷）。
> - Center Channel 由 `calcVHTCenterChannel()` 動態計算（如 ch36/80M→center=42, ch36/160M→center=50）。
> - `HT40+` 或 `HT40-` 由 `calcHT40Mode()` 根據頻道號動態決定。

#### 2.4 GHz 頻段

| 組合標籤 | 核心參數產出 |
| :--- | :--- |
| **2.4G be-40M** | `hw_mode=g`, `ieee80211be=1`, `ieee80211ax=1`, `ieee80211n=1`, 動態計算 `HT40+` 或 `HT40-` |
| **2.4G be-20M** | `hw_mode=g`, `ieee80211be=1`, `ieee80211ax=1`, `ieee80211n=1`, `ht_capab=[HT20]` |
| **2.4G ax-40M** | `hw_mode=g`, `ieee80211ax=1`, `ieee80211n=1`, 動態計算 `HT40+` 或 `HT40-` |
| **2.4G ax-20M** | `hw_mode=g`, `ieee80211ax=1`, `ieee80211n=1`, `ht_capab=[HT20]` |
| **2.4G n-40M**  | `hw_mode=g`, `ieee80211n=1`, 動態計算 `HT40+` 或 `HT40-` |
| **2.4G n-20M**  | `hw_mode=g`, `ieee80211n=1`, `ht_capab=[HT20]` |

> **2.4GHz 注意事項**：
> - 2.4G 不應啟用 `ieee80211ac=1` (VHT)。
> - 40M 頻寬的 `[HT40+]` 或 `[HT40-]` 必須由 `calcHT40Mode(channel)` 動態決定，否則頻道溢出會導致 hostapd 崩潰。
> - **WMM 強制依賴**：在 `hw_mode=g` 下啟用 802.11n (`ieee80211n=1`) 時，必須確保輸出 `wmm_enabled=1`，否則部分網卡會因不符規範降速至 802.11g (54Mbps)。

#### Sub-GHz 頻段

| 組合標籤 | 核心參數產出 |
| :--- | :--- |
| **Sub-GHz ah-2M** | `hw_mode=a`, `ieee80211ah=1`, `s1g_prim_chwidth=2` (限 FGH200M HaLow 模組) |

---

### 3.2 `wpa_supplicant.conf` 變更（STA 模式）

在 `network={ ... }` 區塊**內部**注入對應限制指令（注意：不是全域參數）：

#### 頻段鎖定（`freq_list`）
`freq_list` 的值**由程式動態產生**，根據 `ChannelLookupDB` + 當前 `CountryCode` 查詢該頻段所有合法頻率：
```ini
network={
    ssid="TestAP"
    psk="password"
    # 範例：鎖定 5GHz (TW 國碼，含 DFS 頻段)
    freq_list=5180 5200 5220 5240 5260 5280 5300 5320 5500 5520 5540 5560 5580 5600 5620 5640 5660 5680 5700 5720 5745 5765 5785 5805 5825
}
```

#### 協定降級（`disable_*`）
```ini
network={
    ssid="TestAP"
    psk="password"
    disable_eht=1      # 禁用 WiFi 7
    disable_he=1       # 禁用 WiFi 6（可疊加）
    disable_vht=1      # 禁用 WiFi 5（可疊加）
}
```

---

## 4. 模組硬體能力動態過濾

讀取當前選擇的模組 `<Capabilities>`：
1. 若模組不包含 `6G`（如 `FG50V-965`），頻段選單不呈現 6 GHz。
2. 若模組 `<MaxBandwidth>` 為 `80`，過濾掉 `160 MHz` 與 `320 MHz` 的所有預設。
3. 若模組不支援 `be`，過濾掉所有 WiFi 7 選項。
4. Sub-GHz 選項僅在模組具備 802.11ah（如 `FGH200M`）時開放。

---

## 5. 前置條件：STA `wpa_editor` 的 Scope 感知重構

### 問題

現有 `wpa_editor.go` 的 `parseConfig()` 用扁平 `map[string]string` 解析整個檔案，**不區分 global scope 和 `network={}` scope**。若直接往 `configMap` 中塞 `freq_list` 或 `disable_eht`，`rebuildConfig()` 會將其寫在 `network={}` **外部**的 `# Additional parameters` 區塊，導致 wpa_supplicant 忽略這些參數。

### 解決方案

在實作 STA 功能前，需重構 `parseConfig()` 和 `rebuildConfig()` 以區分兩層 scope：

```go
type WpaNetwork struct {
    SSID   string
    Params map[string]string
}

type WpaConfig struct {
    GlobalParams map[string]string   // ctrl_interface, update_config, ...
    Networks     []*WpaNetwork       // 支援多個 network 區塊
}
```

- `parseConfig()` 遇到 `network={` 時建立一個新的 `WpaNetwork` 並切換 scope 收集。
- `rebuildConfig()` 迭代重建所有 `network={...}` 區塊。
- 「頻段鎖定」和「協定限制」的參數注入，**預設作用於第一個 network 區塊**。若解析發現完全無 network 區塊，則自動建立一個帶有 Dummy SSID 的測試區塊再注入。

### 重複項處理策略

現有 `rebuildConfig()` 已包含去重邏輯（`updatedKeys` map 跳過原始檔案中的重複行）。重構後需**在每個 network block 內獨立保留此去重能力**：

1. **`parseConfig()` 去重**：同一個 `network={}` 區塊內出現重複 key 時，**後者覆蓋前者**（`map[string]string` 的天然行為）。
2. **`rebuildConfig()` 去重與保留**：
   - 重建時維護獨立的 `writtenKeys` set，確保不會在同一個 `network={...}` 內輸出兩行相同的 key。
   - **保留其餘自訂參數**：用戶原有的 `bssid`、`priority`、`scan_ssid` 等非精靈管轄之參數，必須原封不動寫回。
3. **精靈注入語意為「取代」(Replace)**：
   - 設定頻段鎖定時，新的 `freq_list` 值**取代**既有值，不追加。
   - 設定協定限制時，`disable_eht` / `disable_he` / `disable_vht` 各自取代既有值。
   - 選擇「全自動」時，對應 key 從 map 中**刪除**（delete），`rebuildConfig()` 不輸出該行。

---

## 6. 關於 `generator.go` 雙重邏輯的處理

目前專案存在兩套 hostapd 生成邏輯：
1. `generator.go` 的 `GenerateHostapdConfig()` — 舊版簡易產生器，由 AP 模式啟動流程直接呼叫。
2. `ap_config_editor.go` 的 `generateHostapd()` — 新版完整產生器，由選單 `[4]` 產出並寫入 `/etc/hostapd.conf`。

**處理方針**：
- 快速精靈的參數填入操作 `APEditorParams`，最終由 `ap_config_editor.go` 的 `generateHostapd()` 產出。
- `generator.go` 的 `GenerateHostapdConfig()` 應在本次迭代中標記為 `// Deprecated`，後續統一由 `ap_config_editor.go` 的邏輯負責。
- 若 AP 模式啟動流程仍有路徑直接調用 `GenerateHostapdConfig()`，需確認是否改為讀取 `/etc/hostapd.conf`。

> **⚠️ `hotspot.go` 的隱性脫鉤問題**：
> `internal/wifi/hotspot.go` 的 `setupHostapdConfig()` 中，若 `ConfigManager` 未找到現有配置檔（`!exists` 分支），會直接呼叫舊版 `GenerateHostapdConfig()` 現場產出配置。此路徑**完全繞過 `ap_config_editor.go`**，導致無 WiFi 7 支援、無 Center Channel 計算、2.4G 缺 `wmm_enabled` 等問題。
>
> **必須修正**：`hotspot.go` 的 `!exists` 分支應改為引導用戶進入選單 `[4]`（AP 配置編輯器），或改為讀取 `/etc/hostapd.conf`（由 editor 預先生成）。不可再直接呼叫 `GenerateHostapdConfig()`。

---

## 7. 代碼改動規劃

```text
QWifiTestTool/
├── internal/
│   ├── config/
│   │   ├── quick_presets.go       # [新增] 預設矩陣定義、能力過濾函數、精靈選單 TUI
│   │   ├── ap_config_editor.go    # [修改] Start() 新增 [4] 精靈入口；generateHostapd() 修正 2.4G 不輸出 ieee80211ac=1；修正 6G 40M op_class
│   │   ├── wpa_editor.go          # [重構] parseConfig/rebuildConfig 區分 global/network scope，保留去重
│   │   ├── wpa_params.go          # [修改] 新增 freq_list / disable_eht / disable_he 參數定義
│   │   └── generator.go           # [修改] 標記 GenerateHostapdConfig 為 Deprecated
│   ├── models/
│   │   ├── module.go              # [可能修改] 若需新增 Capabilities helper 方法
│   │   └── channel_lookup.go      # [修改] buildChannelKeys() 新增 6G 80M/160M/40M 專屬 case
│   ├── wifi/
│   │   └── hotspot.go             # [修改] setupHostapdConfig() 的 !exists 分支改為引導 editor，不再呼叫舊版 generator
│   └── i18n/
│       ├── zh.go                  # [新增] 快速預設精靈中文提示文案
│       └── en.go                  # [新增] 快速預設精靈英文提示文案
└── docs/
    └── protocol_bandwidth_preset_design.md  # 本設計文檔
```

### 實作優先級建議

| 階段 | 內容 | 難度 |
|---|---|---|
| **Phase 0** | 前置 Bug Fix：`buildChannelKeys()` 6G key 修正 + `generateHostapd()` 6G 40M `op_class=132` 修正 | 低 |
| **Phase 1** | AP 快速精靈（`quick_presets.go` + `ap_config_editor.go` 選單整合 + 2.4G `ieee80211ac` 修正） | 低 |
| **Phase 2** | `wpa_editor.go` scope 感知重構（前置條件） | 中 |
| **Phase 3** | STA 頻段鎖定與協定限制（`wpa_params.go` + `wpa_editor.go` 整合） | 中 |
| **Phase 4** | `generator.go` 統一與 Deprecated 清理 + `hotspot.go` 脫鉤修正 | 低 |

> **過渡期注意**：Phase 1 上線後 Phase 3 完成前，STA 的 `wpa_editor` 尚未支援 scope 感知。若用戶手動在 `[2] 編輯 wpa_supplicant.conf` 中加入 `freq_list` 或 `disable_eht`，`rebuildConfig()` 會將其寫在 `network={}` **外部**，wpa_supplicant 會忽略。Phase 1 發布時應在 UI 中標注「STA 頻段鎖定功能將於後續版本提供」。
