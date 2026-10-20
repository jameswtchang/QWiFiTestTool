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

---

## 7. 最新更新 (2026-10-01) — V1.12 (Quectel FGH200M HaLow AP/STA 鏈路與 Channelization Scheme 閉環修復)

### Morse Micro 前置驅動參數支援 (`PreloadInsmodArgs`)
* **背景**：FGH200M 採用 `dot11ah.ko` 作為前置模組，主驅動為 `morse.ko`。
* **根本衝突**：`dot11ah.ko` 擁有模組參數 `channelization_scheme`（專用於 AU 等地區的頻段方案劃分）。若載入時未帶參數，內核預設為 `3` (IEEE802.11-REVmf / US 方案)。然而 AP 設定檔要求 `country_code=AU` 與 `op_class=70`（要求 Scheme `1`），導致 `hostapd_s1g` 查詢驅動時直接崩潰：`Driver channelization scheme (3) does not include configured Op class (70)`。
* **架構擴充**：
  * 在 `internal/models/module.go` 的 `KernelConfig` 新增 `PreloadInsmodArgs` 欄位與 `GetPreloadArgsList()` 方法。
  * `internal/wifi/driver_loader.go` 載入 `PreloadKo` 時，將對應的 insmod 參數（如 `channelization_scheme=1`）傳入 `findAndInsmod`。
  * 在 `assets/default_modules.xml` 與 `internal/models/default_modules.xml` 中，將 FGH200M 的載入參數修正為 `PreloadInsmodArgs: channelization_scheme=1` 與 `InsmodArgs: country=AU bcf=bcf_default.bin`。

### AP 模式啟動前驅動智能校準
* **防呆機制** (`internal/wifi/hotspot.go`)：
  * 啟動 AP 前加入 `ensureMorseChannelizationScheme()`。讀取 `/sys/module/dot11ah/parameters/channelization_scheme` 與設定檔中的國家碼。
  * 若當前驅動與 AP 要求的 Scheme 不一致，工具自動卸載並以正確 scheme 重新載入，徹底免除使用者手動排查與重載驅動的負擔。

### AP 配置生成器 `op_class` 輸出保留
* **修復** (`internal/config/ap_config_editor.go`)：
  * 先前版本將 `op_class` 列為全局過濾，導致 Sub-GHz 的 `op_class=70` 被吃掉而引起 `invalid op_class=0` 錯誤。
  * 現在僅針對 6GHz 動態生成時過濾，Sub-GHz 及其他頻段完整保留 XML 中的 `FixedParams`。

### Sub-GHz `morse_cli` 硬體頻率與速率原生解析
* **整合** (`internal/wifi/connector_helpers.go`)：
  * 解決 nl80211/iw 將 802.11ah 映射到 5GHz 假象頻率（如 5805 MHz）的問題。
  * 接入 `morse_cli -i wlan0 channel` 與 `morse_cli -i wlan0 stats`，真實解析 920.5 MHz 頻率、8MHz 頻寬與 MCS/PHY 速率。

---

## 8. 核心組件：Channel Lookup 國家頻段檢索庫全面測試與驗證

### 測試覆蓋範圍 (`internal/models/channel_lookup_test.go`)
* **跨國多頻寬矩陣驗證**：針對 US、CN、TW、JP 等主流國家，在 2.4G/5G/6G 各頻寬（20M, 40M, 80M, 320M）進行可用信道查詢與合法性檢驗。
* **AP 類型退避測試**：驗證 `室内型AP`、`室外型AP` 與 `所有类型AP` 的法規信道差異（如 JP 室外限制 W56、CN 室外僅 149、TW 6G 320M 僅中心頻道 31）。
* **格式解析邊界測試**：涵蓋連號範圍（`1-13`）、離散列表（`36, 40, 44`）、混合字串、空值及連字號（`-`）容錯處理。
* **執行結果**：全數通過單元測試 (`go test -v ./internal/models`)。

---

## 9. 最新更新 (2026-10-07) — V1.13 (DFS CAC 狀態機閉環、Realtek 5GHz DFS 依賴修正、全量 29 組 AP 實體射頻測試與測試端健壯化)

### DFS 頻道可用性檢查 (CAC) 狀態機閉環
* **背景**：5GHz 160MHz 或 DFS 頻道啟動時，內核與法規要求進行 60 秒 CAC 掃描。先前版本僅依賴 `iw dev <iface> info` 的 `type AP` 判定，但 CAC 期間介面類型早已切換為 `type AP`（尚未廣播 Beacon），導致 75 秒超時形同虛設並產生假 PASS。
* **閉環實現** (`internal/wifi/hotspot.go`)：
  * 新增 `dfsCACPending()` 檢驗機制，優先透過 `hostapd_cli status` 查詢 `state=ENABLED`。
  * 若無 CLI 工具則解析 hostapd 內部日誌，監聽 `DFS-CAC-START`、`DFS-CAC-COMPLETED success=1` 與 `AP-ENABLED` 事件，確保 CAC 60 秒徹底完成並正式發射 Beacon 後才交棒進入就緒狀態。

### Realtek 模組 5GHz DFS 依賴修正
* **修復** (`internal/config/ap_config_editor.go`)：
  * hostapd 配置檢驗強烈要求 `ieee80211h=1` 必須伴隨 `ieee80211d=1` 與 `country_code`，否則直接退出。
  * 修正前 Realtek 因由驅動處理法規域而略過 `country_code`，造成 DFS 頻道 hostapd 無法啟動。現在針對 DFS 頻道一律補齊該依賴鏈，並由單元測試（TestNewPresetsGenerateHostapd / 10b）鎖定防護。

### E2E 自動化測試套件健壯化與全量 29 組 AP 射頻驗證
* **修復** (`scripts/e2e_wifi_test.ps1`)：
  * 修復 PowerShell 5.1 雙引號字串截斷導致 STA `disable_ht40` 相容性探測永為 0 的問題。
  * 修正 UTF-8 with BOM 編碼，解決 Windows PowerShell 中文正則比對失敗。
  * 動態擴展 DFS CAC 測試超時為 120 秒，並收窄錯誤訊息正則匹配。
* **實測成果**：
  * 在 Qualcomm FME170Q-865 實體板端執行全量 29 組 AP 實體射頻測試 (`-RfAll`)，達成 **29 / 29 PASS (100% 通過率)**。

### STA / AP 連線資訊結構化排版重構 (UI Visual Hierarchy)
* **重構** (`internal/wifi/connector_helpers.go`, `connector_direct.go`, `hotspot.go`)：
  * **三層視覺架構**：將平鋪混亂的終端輸出重整為「基礎狀態與認證 (Basic/Auth)」、「射頻與鏈路品質 (RF/PHY)」、「網路層 (Network)」三組結構化區塊。
  * **STA 聚焦下行指標**：STA 模式下行依賴 RSSI（dBm 及優良評級）與物理調變速率（Bitrate/MCS），去除無效且易混淆的發射功率 (TxPower)。
  * **AP 資訊純淨化**：移除 AP 啟動後舊有的冗餘重複行（介面/驅動/SSID），將實體射頻頻道、頻寬（含 6GHz 320M 識別）與硬體 TxPower 整合至 RF 群組統一輸出。

---

## 10. 最新更新 (2026-10-12) — V1.14 (雙軌設定雙向統一、CAC 即時訊號響應、驅動參數運行時校驗、PATH 盲區多層搜尋與架構加固)

### 10.1 雙軌設定檔生成機制收斂與 WPA3 自動防護
* **背景**：先前 `generator.go`（後台自動生成）與 `wpa_editor.go`（互動式 TUI 編輯器）存在邏輯分歧：後台模式會丟失 XML 預設 network 參數；TUI 編輯器手動拼裝字串容易造成格式不一致；且 WPA3-SAE 模式遺漏強制性 Protected Management Frames (PMF)。
* **架構統一** (`internal/config/generator.go`, `wpa_editor.go`)：
  * `wpa_editor.go` 的預設配置生成直接委託調用 `GenerateWpaSupplicantConfig()`，實現全系統單一真理源（Single Source of Truth）。
  * 支援讀取 XML 中的 `network_` 前綴參數（`network_ssid`, `network_key_mgmt`, `network_sae_password`, `network_psk`），優先採用傳入參數，缺省時由 XML 自動補齊。
  * 偵測到 `SAE` 認證時，自動注入 `ieee80211w=2` (PMF 強制)；若未指定預設 SSID，保證不輸出無效的空 `network={}` 區塊，防止 `wpa_supplicant` 解析報錯退出。

### 10.2 行程與訊號管理對稱性 (Ctrl+C 優雅中斷與資源防護)
* **DFS CAC 等待即時響應** (`internal/wifi/hotspot.go`)：
  * 將信號通道 `sigChan` 提早註冊至服務啟動階段，在 60 秒 CAC 等待循環中透過 `select { case <-sigChan: ... }` 監聽。
  * 使用者隨時可按 Ctrl+C 即刻停止 AP 服務並清理資源，徹底消除長達 60~75 秒的阻塞 hang 現象。
* **STA 模式進程追蹤與路由隔離** (`internal/wifi/connector.go`, `connector_disconnect.go`)：
  * `Connector` 實例新增 `wpaCmd *exec.Cmd` 成員，退出時直接調用 `cmd.Process.Kill()`，杜絕殭屍行程。
  * 退出清理時追加 `ip route flush dev <iface>` 與 link down，嚴格局限於指定無線網卡，絕不波及系統的 `eth0` 乙太網。

### 10.3 驅動健全性檢查 (Sanity Check) 強化與動態參數比對
* **多模組依賴支援** (`internal/wifi/diagnosis.go`)：
  * Sanity Check 擴充支援 `module.Kernel.GetPreloadKoList()`，逐一核對底層模組（如 pcie, s1g 等）載入狀態，並自動排除平台 built-in 模組（`pcie-dw-*`）誤報。
  * 擴充 `/dmesg` 搜尋關鍵字，同時監控主驅動與 Preload 依賴模組的內核日誌。
* **執行期參數比對與重載提示** (`internal/wifi/diagnosis.go`, `driver_loader.go`, `cmd/main.go`)：
  * 深入檢驗 `/sys/module/<modName>/parameters/`，比對內核實際運行值與 XML 期望值。
  * 偵測到參數不一致時，主動提示使用者「重新卸載並載入驅動以套用正確參數」。
  * `UnloadDriver` 卸載前先主動停用網路介面與殺除佔用行程，消除 `Module is in use` 卸載失敗。

### 10.4 可執行檔多層級搜尋策略 (消除 PATH 盲區)
* **四層動態解析架構** (`internal/sys/cmdpath.go`, `binary_mgr.go`, `hotspot.go`)：
  1. `exec.LookPath` (搜尋當前 `$PATH`)
  2. 檢查當前執行檔同級目錄 (`filepath.Dir(os.Executable())`)
  3. 掃描標準系統目錄 (`/usr/sbin`, `/sbin`, `/usr/local/bin`, `/system/bin`, `/data/local/tmp` 等)
  4. 驗證 fallback 預設路徑存在性
* **專用二進制動態對接**：
  * `BinaryManager` 新增 `GetWpaSupplicantPathFor(module)` 與 `GetHostapdPathFor(module)`，自動為 Morse Micro HaLow 模組對接 `wpa_supplicant_s1g` / `hostapd_s1g`。
  * AP 模式動態解析 `dnsmasq` 完整路徑，徹底解決 Buildroot / Busybox / Android 環境下因為普通使用者無 `/usr/sbin` 導致 `command not found` 的問題。

### 10.5 開發維護陷阱消除：XML 單一真理源重構 (package assets embed)
* **背景與痛點**：
  * 過去受限於 Go `//go:embed` 不允許存取包含 `..` 的上層路徑限制，專案在 `internal/models/` 複製了另一份 `default_modules.xml` 並依賴腳本同步。
  * 若工程師在 IDE / VS Code 直接按 Run / Debug，或在終端機下 `go build` / `go test`，不會觸發 `build.bat` 複製指令，導致修改 `assets/default_modules.xml` 後二進制檔案嵌入的仍是舊版配置，陷入「改了設定卻不生效」的幽靈狀態。
* **重構實現** (`assets/modules.go`, `internal/models/loader.go`)：
  * 建立 `package assets`（`assets/modules.go`），在與 XML 同級目錄下執行 `//go:embed default_modules.xml` 導出 `assets.DefaultModulesXML`。
  * `internal/models/loader.go` 直接 `import "qwifitesttool/assets"` 讀取內嵌字節，徹底刪除 `internal/models/default_modules.xml` 實體檔案，全倉庫只保留唯一一份真理源。
  * 保留雙軌防護：開發板 `/data/local/tmp/default_modules.xml` 存在時優先載入（現場調校免重編譯）；不存在時無縫回退至內嵌配置（保證不怕漏推檔案）。
  * 增加版本漂移防呆提示：若板端外部檔案與內嵌預設版本內容不同，自動輸出 `[INFO] 偵測到自訂外部配置 (與程式內嵌版本不同)`，防止舊檔案遮蔽新版本行為。
  * 淨化工作流：移除 `build.bat`、`build.sh` 與 `.agent/workflows/deploy.md` 內的 XML 複製包袱。

---

## 11. 最新更新 (2026-10-14) — V1.15 (支援模組晶片型號全面校準對齊與文檔規範化)

* **晶片型號全面校驗與文檔規範化** (`README.md`, `internal/sys/README.md`, `assets/android_arm64/README.md`)：
  * 依據 `assets/default_modules.xml` 定義，全面校準全系統 11 款模組的晶片型號、最高協定與內核驅動檔名。
  * 修正過去文檔中錯誤標註的晶片型號（如 FCM360W 修正為 MediaTek MT7663、FC21 修正為 Realtek RTL8821CU、FCU741R 修正為 Realtek RTL8733BU、FCS961K-L 修正為 Aicsemi AIC8800、FC905A 修正為 Infineon CYW43455 等）。
  * 在 `internal/sys/README.md` 與 `assets/android_arm64/README.md` 建立標準化「支援模組與晶片型號」對照表，方便工程師與現場測試人員快速對照。

## 12. 最新更新 (2026-10-15) — V1.16 (網卡範圍行程清理、中文 SSID 連線、Android 平台修正與 Claude Code 工作流)

### 12.1 網卡範圍行程清理 (`internal/sys/proc_iface.go`)
* **問題**：STA/AP 啟停與驅動卸載原本對 `wpa_supplicant`、`hostapd`、`dnsmasq`、`udhcpc`、`dhclient` 全域 `killall -9`，會連帶殺掉 eth0 的 DHCP client (租約到期斷網) 與其他網卡的服務。
* **機制**：讀取 `/proc/<pid>/cmdline` 與 daemon 設定檔判定綁定網卡：
  * DHCP client：僅殺 `-i <iface>` / 參數含 `<iface>` 者。
  * `wpa_supplicant` / `hostapd`：殺綁定目標網卡者，以及未宣告網卡者 (D-Bus 模式、設定檔已刪除)；明確只綁其他網卡者保留。hostapd 解析位置參數設定檔與 `-i` / `-b`。
  * `dnsmasq`：宣告介面 (`-i`、`interface=`) 含目標網卡，或完全未宣告介面 (wildcard / 僅 `listen-address`，如 FGH200M 的 `dnsmasq_bind_interfaces=0`) 即清理，避免殘留行程佔住 DHCP/DNS port。
  * 改以 argv[0] 完整名稱比對，修正 `killall` 只比對 15 字元 comm、`wpa_supplicant_s1g` 僅靠 `pkill` 模糊比對誤中的問題。
* Android framework 接管 (`service.go`) 維持全域處理。

### 12.2 中文 / 特殊字元 SSID 與密碼校驗 (`internal/wifi/wpa_string.go`)
* `wpa_cli scan_results` 改以 tab 切欄 (非 tab 輸出自動退回舊解析)，並解碼 `printf_encode` 跳脫 (`\xNN`、`\"`、`\\`、八進位)。
* 非可列印 ASCII 的 SSID 以 hex (`ssid=e4b8ad...`) 寫入設定檔；WPA-PSK 密碼須 8–63 字元或 64 位 hex (hex 不加引號)，不合法時於停止 daemon 前即拒絕。
* `freq_list` / `disable_*` 策略參數輸出順序固定。

### 12.3 其他修正
* AP 子網路遮罩改以 `net.IPMask` 換算任意合法遮罩，非法遮罩直接報錯 (原本僅識別 /24、/16)。
* 驅動卸載時「未載入」誤顯示為「已載入」的訊息修正 (`drv.not_loaded`)。
* `iw` 改經 `FindCommand` 解析路徑。

### 12.4 Android 平台修正 (尚無 Android 板，未實機驗證)
* 內嵌 `wpa_supplicant` / `hostapd` / `iw` 目前為文字佔位檔：解壓前檢查 ELF magic，無效時改用系統執行檔 (`/vendor/bin/hw/` 等) 並刪除舊版遺留佔位檔。
* 無 `iw` 時以 `hostapd_cli status` (`state=ENABLED`) / `AP-ENABLED` 日誌判定 AP 就緒。
* 啟動 daemon 前停用 Android WiFi framework，結束時還原 (保留 STA 連線或原本即關閉時不還原)。
* 已知未處理：Android「Wi-Fi 掃描一律開啟」可能讓 framework 以 scan-only 模式保留 wpa_supplicant。

### 12.5 E2E 測試與工作流
* 新增 `scripts/qwt_board_cleanup.sh` (POSIX sh，規則與 12.1 一致)，E2E 測試啟動時自動推送；清理與 RF teardown 斷言改為只看測試網卡。
* 新增 Linux 實體行程整合測試 (`internal/sys/proc_iface_linux_test.go`)，以假 daemon 驗證 Go 實作與 sh 腳本選出的 PID 一致。
* 新增 Claude Code skills (`.claude/skills/`：`/deploy`、`/push`、`/sync`、`/verify-hardware`)，`.agent/workflows/` 保留。

### 12.6 驗證成果 (Qualcomm FME170Q-865，Linux 5.10.160)
* 單元測試全數通過；板端整合測試 16/16 PASS。
* E2E：Phase 1 AP 矩陣 29/29、Phase 2 STA 策略 11/11 (含實體 RF 掃描)、Phase 3 核心 RF 6/6 PASS。
* 4 個綁定 eth9 的對照 daemon (udhcpc / dnsmasq / wpa_supplicant / hostapd，comm 與真實名稱相同) 於完整 E2E 全程存活。

## 13. 最新更新 (2026-10-16) — V1.17 (企業版 AP 明確標示、英文介面 i18n 補齊、冗餘程式清理與 /sync 流程強化)

### 13.1 企業版 802.1X (EAP) AP 處理 (`internal/wifi/wpa_string.go`, `scanner.go`, `connector.go`)
* **問題**：`deriveKeyMgmt` 將 `[WPA2-EAP-CCMP]` 判為 `WPA-PSK`、掃描清單標為「WPA/WPA2-Personal」，連線必定失敗且錯誤訊息引導使用者檢查密碼；密碼留空時更會寫成 `key_mgmt=NONE` (開放網路)。
* **修正**：新增 `isEnterpriseOnly`（含 `EAP` 且無 `PSK` / `SAE` 替代）；掃描清單標為 `WPA2-Enterprise (802.1X)`，選取後於輸入密碼前停止並提示 (`conn.eap_unsupported`)；`buildNetworkBlock` 於函式開頭即校驗，任何密碼都不會寫出錯誤設定。EAP+PSK 混合 AP 仍以 PSK 連線。
* 完整 EAP (PEAP / TTLS 帳密) 支援不在本版範圍。

### 13.2 英文介面 i18n 補齊
* `hotspot.go` (12)、`sys/platform.go` (5)、`main.go` (2)、`models/loader.go` (1) 共 19 行寫死中文改走 i18n，新增 20 個 key (zh / en)。
* 國家頻道 JSON 的欄位名 (`所有类型AP`、`信道` 等) 為資料鍵，維持原樣。

### 13.3 冗餘程式清理
* 移除無呼叫者的 `GenerateHostapdConfig` (已標 Deprecated，91 行)。
* 三份 `isAndroid()` (`models` / `sys` / `config`) 收斂為新套件 `internal/platform` 的 `IsAndroid()`；因 `sys` 依賴 `models`，共用函式需獨立為無依賴套件。

### 13.4 調查結論 (未修改)
* **外部 XML 權限與 `PostLoadCmd`**：外部 XML 設計上即供現場工程師自由修改，維持現狀。
* **PCIe 模組等待網卡名稱**：板上 Qualcomm 網卡 `device/driver` 指向 `cnss_pci` (非 `wlan`)，`GetInterface()` 依驅動偵測對 CNSS 模組恆退回 XML 名稱，等待邏輯改用它不會改變行為；待遇到介面名稱不一致的板子再新增 `<NetDriver>` 欄位。

### 13.5 工作流
* `/sync` 新增版本號檢查與自動遞增、MD 文檔對照表與一致性檢查、程式修改時必須通過實機 E2E 才可提交 (`--quick` / `--skip-hw` 參數)；`AGENTS.md` 增列 `/sync` 驗證步驟可部署的例外。

### 13.6 驗證成果 (Qualcomm FME170Q-865，Linux 5.10.160)
* 本機 `go vet ./...`、`go test ./...` 全數通過 (含新增 `TestEnterpriseDetection`、`TestParseScanResults_SpaceFallback`、`internal/platform` 測試)。
* 實機 E2E (`scripts/e2e_wifi_test.ps1` 完整模式) **46/46 PASS**：Phase 1 AP 矩陣 29/29、Phase 2 STA 策略 11/11、Phase 3 RF 實體廣播 6/6。
* 未涵蓋：企業版 AP 實連 (現場無 802.1X AP)、英文介面字串顯示 (E2E 不比對文字)、Morse 模組訊息、Android 平台；6GHz STA 鎖頻掃描 0 BSS (附近無 6G AP，邊界無法判定)。

## 14. 最新更新 (2026-10-19) — V1.18 (E2E 測試框架強化與覆蓋擴充，含驅動重載已知問題)

本版僅修改測試腳本與文檔，工具程式行為不變 (版本號依 `/sync` 規則因 `scripts/` 修改而遞增)。

### 14.1 測試可信度 (`scripts/e2e_wifi_test.ps1`)
* **依提示節奏送出輸入**：背景執行緒持續讀取 TUI 輸出，等輸出靜止 300ms 才送下一筆；每個 session 必須以「再見!」正常退出，出現「無效」即判為輸入與選單脫節。改版後立即抓到舊腳本 Phase 1 每次多送一個 `0` (存檔後已直接回主選單)。
* **AP 矩陣逐行精確比對**：hostapd.conf 以 `key=value` 比對完整值，並要求未列入預期的模式 / 頻寬相關 key 不存在；預期值由 generator 對 FME170Q 29 組 preset 的實際輸出導出。20M / 40M 互換、WiFi 世代誤標都會判 FAIL (舊版只比對子字串，20M 與 40M 的檢查內容完全相同)。
* **結果狀態**：新增 `INCONCLUSIVE` (設定正確但 RF 掃描 0 BSS)，不再計入 PASS；有 FAIL 時 exit code 為 1，並輸出 `results.json`。
* **失敗證據**：FAIL 自動存下完整 TUI 畫面、當下設定檔、dmesg / `iw` 狀態。
* **板端保護與參數化**：`/etc/wpa_supplicant.conf`、`hostapd.conf`、`dnsmasq.conf` 開跑前備份、結束時 (含 Ctrl+C) 還原；模組依名稱查找 (`-ModuleName`)；網卡改為 `-Iface` 參數；AP 失敗判斷改比對實際存在的錯誤字串。
* **stdin BOM**：.NET 於 `Process.Start` 以 `Console.InputEncoding` (UTF-8 帶 BOM) 建立子行程 stdin，第一筆輸入變成 `﻿y`，語言選擇因此被忽略；腳本改設為不帶 BOM 的 UTF-8。

### 14.2 覆蓋擴充
* **網卡隔離 (TC-SYS-01，`scripts/qwt_canary.sh`)**：在 `eth9` 上啟動 udhcpc / dnsmasq / wpa_supplicant / hostapd 假 daemon，先以清理腳本 dry-run 做正向對照，之後每個 Phase 結束確認存活，涵蓋工具本身與測試清理兩條路徑。
* **英文介面 (TC-AP-EN01 / TC-STA-EN01 / TC-RF-EN01)**：語言選英文後跑完整流程，語言提示之後的畫面不得出現中日韓文字。
* **診斷與驅動生命週期 (`-DiagOnly`，opt-in)**：診斷全通過、卸載後由診斷提示載入、直接重新載入、重載後實際起 AP。

### 14.3 已知問題 (尚未修正)
* **重新載入驅動後 AP 無法啟動 (FME170Q-865)**：工具為避免 kernel hang 只重載 `wlan.ko` (保留 `wlan_cnss_core_pcie` / `pcie-dw-rockchip`)。重載後 hostapd 可達 `AP-ENABLED`，但 `iw` 回報 txpower 持續 `0.00 dBm`，PCIe 模組的就緒判斷逾時而判定失敗 (TC-DIAG-04 穩定 FAIL)。之後模組卡在異常狀態，暖開機後 PCIe 仍 `Link Fail`，需完全斷電才能恢復；因此 Phase 4 不納入預設完整測試。
* 診斷畫面「核心日誌 (最近 5 條)」實際列出 8 條。

### 14.4 驗證成果 (Qualcomm FME170Q-865，Linux 5.10.160)
* 本機 `go vet ./...`、`go test ./...` 全數通過。
* 實機 E2E (V1.18 完整模式，exit code 0)：**49 PASS / 0 FAIL / 1 INCONCLUSIVE (共 50)**。Phase 1 AP 矩陣 30/30 (含英文 1)、Phase 2 STA 策略 11/12 PASS + 1 INCONCLUSIVE (TC-STA-P01 6GHz 鎖頻掃描 0 BSS，附近無 6G AP)、Phase 3 RF 實體廣播 7/7 (含英文 1)、網卡隔離 TC-SYS-01 PASS (eth9 四個 canary 撐過 Phase 1～3)。
* `-DiagOnly` (另行執行，V1.17 執行檔)：TC-DIAG-01～03 PASS，TC-DIAG-04 FAIL (見 14.3 已知問題)。
* 未涵蓋：Station 實際連線與中文 SSID (需參考 AP)、企業版 AP 實連、Android 平台。

## 15. 最新更新 (2026-10-20) — V1.19 (Station 連線保留與設定持久化修正、E2E 實際連線測試)

以 iPhone 熱點 (中文 SSID、WPA3-SAE、2.4GHz) 作為參考 AP 新增 Phase 5 實際連線測試，找出 3 個 Station 模式缺陷並修正。這 3 個缺陷都在一般手動測試較少走到的路徑上：沒有系統設定檔、離開工具後保留連線、帶著既有連線再次進入。

### 15.1 Station 模式修正
* **離開工具後連線中斷 (`connector.go`)**：
  * 原因：wpa_supplicant 的輸出經 pipe 交給工具加時間戳寫入 log。使用者選擇保留連線並離開工具後，pipe 沒有讀取端，daemon 下一次寫 log 就被 SIGPIPE 終止，畫面上卻顯示「連線已保持」。
  * 修正：改以 `sh -c 'trap "" PIPE; exec wpa_supplicant ...'` 啟動，忽略 SIGPIPE 的設定在 exec 後保留，寫入失敗只會回傳 EPIPE；另以 `Setsid` 開新 session (`internal/sys/detach_linux.go`)，不受終端機 SIGHUP / SIGINT 影響。Ctrl+C 與主動斷線仍經 `Stop()` 結束 daemon。
  * Log 格式維持 `[YYYY-MM-DD HH:MM:SS.mmm]`；離開工具後 daemon 的後續輸出不再寫入 log。
* **無系統設定時網路無法保存 (`connector.go`、`config_updater.go`)**：
  * 原因：沒有 `/etc/wpa_supplicant.conf` 時，工具使用工作目錄的 `wpa_supplicant_<module>.conf`，但每次進入都把它重寫成空白範本，上次連過的網路因此遺失，自動重連永遠不會成功。
  * 修正：仍每次依 XML 重建，讓 XML 修改生效，但保留上次存的 network 區塊，並顯示「已保留先前儲存的網路設定」。
  * 選擇「保存為客製化配置」後，之後新增的網路會寫入客製化檔 (原本客製化檔只會存下空白範本)。
* **既有連線的中文 SSID 顯示為 `\xNN` (`connector_direct.go`)**：`iw dev link` 的 SSID 同樣有跳脫，改用 `decodeWpaString` 解碼。
* **防護 (尚未實機驗證)**：
  * 修正後 wpa_supplicant 會存活到工具結束之後，因此 `startDaemon` 啟動前會先結束同一張網卡上殘留的 wpa_supplicant，避免兩個實例同時操作網卡。
  * Station 流程出錯時，改為停掉本次啟動的 daemon，不再留在背景。

### 15.2 E2E Phase 5 (`-StaConnect` / `-StaConnectOnly`)
* 參考 AP 資訊放在 `%USERPROFILE%\.qwt\ref_ap.txt` (第 1 行 SSID、第 2 行密碼)，不進 repo；主控台、`results.json`、TUI 紀錄中的密碼一律遮罩為 `***`。
* 測試項目：
  * TC-STA-C01 掃描；
  * C02 連線 + DHCP + gateway ping，並檢查非 ASCII SSID 以 hex 寫入；
  * C03 斷線無殘留；
  * C04 自動重連 (第 2 個缺陷的迴歸測試)；
  * C05 保留既有連線 (第 1、3 個缺陷的迴歸測試)；
  * C06 錯誤密碼回報失敗。
* 開跑前先以 `iw scan` 等待參考 AP 出現 (最多 90 秒)；手機熱點閒置時會停止廣播，看不到 AP 時判 INCONCLUSIVE。前置項目未通過時，後續項目判 INCONCLUSIVE，不連帶判 FAIL。
* 工具在工作目錄產生的 `wpa_supplicant_*.conf` 納入板端備份與還原，避免參考 AP 密碼留在板上。

### 15.3 已知問題 (尚未修正)
* 工具的 session log 與設定檔顯示會以明文記錄 `sae_password` / `psk`。
* 14.3 的驅動重載問題、診斷畫面核心日誌條數問題仍存在。

### 15.4 驗證成果 (Qualcomm FME170Q-865，Linux 5.10.160)
* 本機 `go vet ./...`、`go test ./...` 全數通過；新增 `carryOverSavedNetwork` 與 iw SSID 格式解碼的單元測試。
* 實機 E2E (V1.19 完整模式，exit code 0)：**49 PASS / 0 FAIL / 1 INCONCLUSIVE (共 50)**。Phase 1 AP 矩陣 30/30、Phase 2 STA 策略 11/12 PASS + 1 INCONCLUSIVE (TC-STA-P01 6GHz 鎖頻掃描 0 BSS，附近無 6G AP)、Phase 3 RF 實體廣播 7/7、網卡隔離 TC-SYS-01 PASS。
* Phase 5 (`-StaConnectOnly`，參考 AP 為 iPhone 熱點，執行檔程式碼與 V1.19 相同，僅版本字串尚未更新)：**7/7 PASS** (C01～C06 + TC-SYS-01)；wpa_supplicant log 時間戳格式確認不變。




