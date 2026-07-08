# QWiFiTestTool - 項目分析與進展紀錄

## 1. 核心問題修復：Realtek 模組 Regulatory (法規網域) 衝突與 NO-IR 限制

### 發生背景
在測試 Quectel FCE863R (Realtek RTL8852BE) 模組的 AP 模式時，嘗試開啟 5GHz 頻段（例如 Channel 36 / 5180MHz），遇到了 `Frequency 5180 (primary) not allowed for AP mode, flags: 0x853 NO-IR` 的錯誤，導致 hostapd 拒絕啟動。

### 根本原因分析
1. **Self-managed 驅動特性**：Realtek 驅動在掛載時透過 `rtw_country_code=CN` 讓它處於 Self-managed 狀態，內部法規套用為 `CN`。
2. **Global cfg80211 預設值**：由於 Linux 核心的全局 (Global) `cfg80211` 法規預設為 `00` (World Regulatory Domain)，此規範下多數 5GHz 頻道被標記為 `NO-IR`（不允許主動發射電磁波）。
3. **hostapd 判斷依據**：`hostapd` 在啟動時會向核心查詢全局法規設定，看到 `00` 後便強制作為 `NO-IR` 阻斷了 AP 的建立。

### 解決方案
1. **恢復並調整 Fallback 機制**：在 `hotspot.go` 中，即使不依賴 `hostapd.conf` 內的 `country_code`，也讓工具能從模組的 `InsmodArgs` (如 `rtw_country_code=CN`) 解析出正確的國家碼。
2. **主動同步全局法規**：在啟動 `hostapd` 之前，主動在作業系統層級透過 `iw reg set CN` 強制將全局法規與驅動法規對齊。這樣一來，全局的 `cfg80211` 就會解鎖 5GHz (移除 NO-IR 限制)，且不會與 Realtek 驅動本身的機制發生衝突。

---

## 2. 功能規劃：hostapd 測試情境設定檔分析

針對 `d:\QWiFiTestTool\hostapd` 資料夾內的預設測試設定檔（包含 a/b/g/n/ac/ax 等各種 20M/40M/80M 組合）進行了深入分析。

### 設定檔潛在風險 (隱藏地雷)
* **`channel=0` (ACS 自動選頻) 的風險**：絕大多數的高速設定檔 (N/AC/AX) 使用了 `channel=0`。這高度依賴環境中的 hostapd/driver 必須支援 ACS (Automatic Channel Selection)。若不支援則會直接啟動失敗，測試時需手動改為固定頻道。
* **非標準參數 `force_40mhz=1`**：出現在 40Mhz 的配置中。這是 Realtek/MTK 等定製版 hostapd 專有的非標準指令，標準開源版 hostapd 無法解析，會導致 `unknown configuration item` 錯誤。
* **80MHz 缺少中心頻段 (Center Frequency)**：AC_5G_80M 與 AX_5G_80M 設定檔啟用了 80MHz (`vht_oper_chwidth=1`)，但未指定 `vht_oper_centr_freq_seg0_idx`。若未使用 ACS，標準 hostapd 預設會報錯。
* **Country Code 衝突風險**：多數檔案寫死了 `country_code=US` 或 `CN`，若直接覆蓋系統配置，可能再度引發 Realtek 模組的法規衝突。

### 整合解決方案：情境範本庫 (Profile Templates)
為了安全且便利地使用這些測試檔，規劃開發 **「情境範本庫」** 功能：

1. **打包範本**：將所有 `.conf` 檔案移至 `assets/hostapd_profiles/`，並透過 Go 的 `go:embed` 機制直接打包進執行檔，免除手動 Push 的麻煩。
2. **新增選單選項**：
   在 AP 配置編輯器的主選單中加入：
   `[3] 從預設情境範本載入 (快速套用 11n/ac/ax 等情境)`
3. **無縫接軌編輯器**：使用者選擇特定範本後，工具會「解析」該範本的內容並覆蓋到當前的記憶體參數狀態 (保留 `force_40mhz` 等特殊參數至 FixedParams 中)。
4. **防呆與預覽**：載入後，透過 QWiFiTestTool 既有的生成與過濾邏輯（例如自動濾除 Realtek 模組的 country_code），最後進入預覽畫面。使用者可以在寫入前做最後的參數微調。

---

## 3. 最新更新 (2026-05-13)

### 新增模組支援：Quectel FCS866R
* **模組定義**：在 `default_modules.xml` 的第三個順位新增 `quectel_fcs866r`。
* **技術規格**：指定使用 `8852bs.ko` (Realtek RTL8852BS Chipset)，並配置與 FCE863R 同等級的高性能參數（支援 11ax, 80MHz 頻寬, 以及 WPA3-SAE 安全加密）。

### UI 與診斷邏輯優化
* **TxPower 數值溢位修復**：針對部分 Realtek 驅動回傳異常 TxPower 數值（如 `42949... dBm`，本質上是 uint32 下溢位）的問題，在 `iw_helper.go` 中實作了攔截過濾機制，改為顯示「未知 (Driver 未提供)」，避免誤導用戶。
* **交互式診斷功能 (Post-Diagnosis Menu)**：
  在 `RunDiagnosis` 所有檢查項目通過後，新增快捷操作選單：
  * `[1] 卸載當前驅動`：手動釋放硬體資源。
  * `[2] 卸載當前驅動, 並重新載入驅動`：快速重置 WiFi 模組狀態。
  * `[0] 返回操作模式`：結束診斷。

### 底層調試與維護性增強
* **Debug 指令透明化**：在 `internal/wifi/driver_loader.go` 中，針對驅動的 `insmod` 與 `rmmod` 操作加入了 `[DEBUG]` 日誌打印。現在會明確顯示執行的完整指令路徑與參數，便於在開發板環境下進行快速排錯。
* **配置同步機制優化**：強化了 `assets/default_modules.xml` 與嵌入代碼 `internal/models/default_modules.xml` 之間的同步邏輯，確保每次編譯都能攜帶最新的模組定義。

---

## 4. 最新更新 (2026-07-08) — V1.01

### 新增模組支援：Quectel FME164Q (Qualcomm QCA2064)
* **模組定義**：在 `default_modules.xml` 新增 `quectel_fme164q`，為首個 Qualcomm 平台模組。
* **技術規格**：支援 WiFi 6 (802.11ax)、2.4G/5G 雙頻段、80MHz 頻寬、WPA3-SAE 安全加密。
* **驅動架構**：使用 `wlan_cnss_core_pcie.ko` (前置) + `wlan.ko` (主驅動) 雙模組載入架構，需 `country_code=TW` 作為 insmod 參數。

### 驅動後置命令機制 (PostLoadCmd)
* **新增 XML 欄位**：`<PostLoadCmd>` 支援在驅動 insmod 完成後自動執行 shell 命令（如 `ifconfig wlan0 up`）。
* **實作方式**：在 `driver_loader.go` 中透過 `exec.Command("sh", "-c", cmd)` 執行，支援管線與條件語法。
* **向下相容**：未定義此欄位的模組 `PostLoadCmd` 為空字串，直接跳過執行區塊，不影響既有行為。

### 6GHz 頻段架構支援
* **頻段偵測**：在 `hotspot.go` 與 `ap_config_editor.go` 中加入 `op_class 131~137` 判斷邏輯，可正確辨識 6GHz 頻段。
* **hostapd 生成**：`generator.go` 新增 6G 模式下自動過濾 HT/VHT 舊參數 (`ieee80211n`, `ieee80211ac`, `ht_capab`, `vht_capab`, `vht_oper_chwidth`, `vht_oper_centr_freq_seg0_idx`)，防止 hostapd 崩潰。
* **HE 中心頻道計算**：`ap_config_editor.go` 新增 `calcHE6GCenterChannel()` 函數，支援 6GHz 80MHz 的 15 個中心頻道索引 (7~231)。
* **備註**：FME164Q 模組目前不支援 6GHz 頻段，此為架構預留，確保未來 6GHz 模組可直接使用。

### UI 終端對齊優化
* **CJK 對齊引擎**：在 `internal/ui/menu.go` 新增 `AlignLabel()` 函數，基於 Unicode East Asian Width 規範計算全形/半形字元的視覺欄位寬度，確保中英文混合標籤完美對齊。
* **影響範圍**：
  - AP 詳細資訊面板 (`hotspot.go`)
  - 當前配置面板 (`ap_config_editor.go`)
  - iw 介面資訊 (`iw_helper.go`)
* **色彩裝飾**：為 `頻道` 與 `發射功率` 欄位加上粗體與綠色/黃色色彩，與面板風格統一。

### 版本號更新
* Banner 版本號更新為 `V1.01`。
