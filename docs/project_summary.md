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

---

## 5. 最新更新 (2026-08-27) — V1.03 (完整 i18n 雙語化與 AP 模式死鎖修復)

### 全系統 i18n 雙語架構實作
* **輕量級自研 i18n 模組** (`internal/i18n`)：
  - 專為嵌入式 ARM64 Linux 設計，不引入肥大第三方依賴包。
  - 採用 Map 表驅動架構，提供 `i18n.T(key)` 與 `i18n.Tf(key, args...)` 格式化輸出。
  - **三層 Fallback 機制**：當前語言 -> 繁體中文預設 -> 原始 Key，確保絕不因缺詞條崩潰。
  - **啟動語言選單**：程式啟動時預設使用繁體中文，提示 `Switch language to English? [y/N]:`，輸入 `Y/y` 切換為英文，其餘維持繁中。
* **100% UI / 流程雙語覆蓋**（新增 210+ 翻譯 Key）：
  - 主選單與 Banner (`cmd/main.go`, `internal/ui/menu.go`)
  - Station 模式連線、掃描與錯誤診斷 (`internal/wifi/connector*.go`, `scanner.go`)
  - AP 熱點模式與系統服務 (`internal/wifi/hotspot.go`, `iw_helper.go`)
  - 驅動掛載與卸載 (`internal/wifi/driver_loader.go`, `diagnosis.go`)
  - 參數與設定檔編輯器 (`internal/config/wpa_editor.go`, `ap_config_editor.go`, `manager.go`)
  - 日誌模組與模組載入器 (`internal/logger/logger.go`, `models/loader.go`)

### AP 模式 Ctrl+C 退出死鎖 Bug 修復
* **問題現象**：在 AP (Hotspot) 模式下按下 `Ctrl+C` 終止服務時，畫面停留在 `kill <pid> (hostapd)` 後永久阻塞，無法退出程式。
* **根本原因分析**：
  1. `hotspot.Start()` 內已啟動背景 goroutine 執行 `h.hostapdCmd.Wait()` 監控 process。
  2. 使用者發送中斷信號後，主執行緒在 `Stop()` 內又調用了 `h.hostapdCmd.Wait()` 與 `h.dnsmasqCmd.Wait()`。
  3. 在 Go 語言 `os/exec` 設計中，同一個 `exec.Cmd` 重複或並發調用 `Wait()` 會因 pipe I/O lock 或重複回收引發死鎖/阻塞。
  4. 此外，信號通道 `sigChan` 在 `Start()` 結束前未釋放，造成後續再次按 Ctrl+C 依然被空通道攔截。
* **解決方案**：
  1. `Stop()` 內改為直接發送 `Process.Kill()`，移除重複的阻塞性 `Wait()`，進程回收交由後續的 `killall -9` 保證清理。
  2. `Start()` 加入 `defer signal.Stop(sigChan)`，確保離開 AP 模式後釋放系統信號監聽。

### 終端排版與 CJK 對齊精細化
* **標籤寬度擴展**：將 AP Details 與 Interface Details 的 Label 寬度從 10 擴大為 16 (`ui.AlignLabel(label, 16)`)。
* **長標籤支援**：解決英文版下 `Country Code:` (13 字元) 和 `Gateway IP:` (11 字元) 超出欄寬導致值對齊錯位的問題，中英文環境下均呈現整齊一致的視覺效果。

### 版本號升級
* Banner 與版本識別更新為 `V1.03`。

---

## 6. 最新更新 (2026-09-01) — V1.05 (DHCP 超時保護、AP 衝突清理、全域靜態分析修復與排版優化)

### Station 模式 DHCP 客戶端阻塞卡死修復
* **問題現象**：在 Station 模式連線成功後，進入 `[INFO] 獲取 IP 地址...` 並執行 `udhcpc -i wlan0 -n -q -t 5` 時，程式無限期阻塞無法完成連線流程。
* **根本原因分析**：
  1. 嵌入式 Linux (BusyBox `udhcpc`) 依賴 `/usr/share/udhcpc/default.script` 設定網卡 IP，若系統缺少預設腳本或 AP 端 DHCP 回應延遲，命令可能無法及時退出。
  2. 原程式採用同步阻塞式 `exec.Command(...).Run()`，未設置執行逾時防護，導致 Go 主流程永久等待。
* **解決方案**：
  1. **導入 Context Timeout 雙重保護**：
     - `udhcpc` 加入 `context.WithTimeout(..., 8*time.Second)` 與優化參數 `-t 3 -T 2`（減少重試輪數）。
     - 若 `udhcpc` 超時或失敗，自動切換至 `dhclient`（附帶 `-1 -v` 單次嘗試模式與 `8 秒` 超時保護）。
  2. **流程不斷裂**：若雙 DHCP 皆未取得 IP，記錄警告日誌並繼續顯示當前已建立的 WiFi 資訊與排錯提示（如提示手動執行 `udhcpc` 或靜態配置），流程安全返回。

### 模組選單排版精細化
* **長名稱對齊**：在 `internal/models/loader.go` 的 `ListModules()` 中，移除不可預期的 `\t` 跳格，改為固定 `%-22s` 空格填充。
* 解決 `Quectel FME170Q-865` 等較長模組名稱在終端機中跳格過度造成晶片型號括號錯位的問題。

### 版本號管理架構重構
* **頂層常數化**：在 `cmd/main.go` 獨立定義 `const Version = "V1.04"`，Banner 顯示時由 `ShowBanner(Version)` 傳入。
* **i18n 樣板化**：`zh.go` 與 `en.go` 中的 `banner.version` 改為使用 `%s` 佔位符，未來版本進版只需修改 `main.go` 單一檔案，大幅降低維護成本。

### AP 模式啟動前衝突進程清理與 AP 就緒檢測
* **問題現象**：在執行過 Station 模式連線後切換至 AP 模式，`hostapd` 報錯 `nl80211: Failed to set channel (freq=6135): -16 (Device or resource busy)`，但程式仍繼續執行並印出殘留的舊 Station 頻道資訊。
* **根本原因分析**：
  1. 背景殘留的 `wpa_supplicant` 仍連線在舊 AP (如 5GHz 5785MHz)，霸佔了無線網卡，導致核心拒絕切換至 6GHz AP。
  2. `startServices()` 輪詢 6 秒若失敗仍未中斷返回，導致誤報 AP 啟動成功。
* **解決方案**：
  1. 在 `startServices()` 啟動前強制清理 `wpa_supplicant` 等衝突 daemon 與舊介面狀態。
  2. 輪詢 6 秒後加入 `apReady` 嚴格檢驗，若未成功轉為 `type AP` 即刻終止並報錯。
