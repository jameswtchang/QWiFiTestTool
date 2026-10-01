# Quectel FGH200M (Morse Micro 802.11ah) 修改與支援記錄

此文件記錄了為了完整支援 Quectel FGH200M (基於 Morse Micro 晶片的 802.11ah Sub-GHz HaLow 模組) 所作的一系列工具架構調整與 Bug 修復。

## 1. 核心程式切換 (Daemon 支援)
* **需求**：FGH200M 因為採用 802.11ah 草案，必須使用其專屬客製化編譯的 `wpa_supplicant_s1g` 與 `hostapd_s1g`，而非系統自帶版本。
* **修改** (`cmd/main.go`)：
  * 在執行 Station 模式與 AP 模式前，程式會檢查 `module.ID == "quectel_fgh200m"`。
  * 如果吻合，將強制將啟動路徑覆寫為 `wpa_supplicant_s1g` 以及 `hostapd_s1g`（呼叫時也會夾帶 `-dd` 開啟偵錯）。

## 2. IPC Socket 路徑修正 (`wpa_cli` 通訊問題)
* **問題**：`wpa_supplicant_s1g` 預設的控制介面路徑通常不是傳統的 `/var/run/wpa_supplicant`，而是 `/var/run/wpa_supplicant_s1g`，這導致了我們預設使用 `wpa_cli` 觸發掃描 (scan) 以及獲取連線狀態時，會出現 `Failed to connect to non-global ctrl_ifname: wlan0` 錯誤。
* **修改**：
  * **動態載入**：在 `cmd/main.go` 中加入讀取 `assets/default_modules.xml` 內定義的 `<StationConfig>` 區塊，擷取 `<Param key="ctrl_interface">`，並直接覆寫 `platformInfo.WpaSupplicantSocket`。
  * **參數傳遞** (`internal/wifi/scanner.go`, `connector.go`, `connector_helpers.go`)：所有用到 `wpa_cli` 的系統呼叫 (包含 `status`, `scan`, `scan_results`, `terminate`)，都強制加上 `-p [socket_path]` 指定正確的背景服務通訊路徑。

## 3. WiFi Mode 與 Driver 卡死修正
* **問題**：Morse Micro 驅動 (morse_sdio) 在模式切換時非常嚴格。若先前開啟過 AP 模式，`hostapd` 當機或未正確還原介面，會導致下一次啟動 Station 模式時，出現 `Operation already in progress (ret=-114)` 以及 `Register frame command failed` 導致崩潰。
* **修改** (`internal/wifi/connector.go`)：
  * 在 `startDaemon` 與 `startDaemonForScan` 被呼叫、真正啟動 daemon 前，加入強制重置機制：
    ```bash
    ip link set wlan0 down
    iw dev wlan0 set type managed
    ```
  * 從核心層面將介面暴力洗回基礎的 Station (Managed) 模式，徹底避免了「AP 模式殘留」引發的驅動層死鎖。
  * 同時修正了掃描模式專用的 `/tmp/wpa_scan.conf`，確保其產生的設定檔寫入正確的 `ctrl_interface` 路徑，而非寫死傳統路徑。

## 4. AP 模式與 Dnsmasq 動態支援
* **問題**：原始設計將 AP 模式的 Gateway 寫死為 `192.168.50.1`，並且 Sub-GHz AP 顯示資訊的判斷邏輯不夠全面，導致顯示的頻道及網路資訊經常錯誤。
* **修改**：
  * **IP 動態寫入** (`hotspot.go`)：`setupInterfaceIP` 改為優先讀取 XML 的 `<Network>` 區塊 (包含 Gateway 與 Netmask 段落) 來計算並設定介面 IP。
  * **動態顯示 UI** (`hotspot.go`)：`showAPInfo()` 現在會 parse 產生好的 `/etc/dnsmasq.conf` 自動抓取 `listen-address` 以及 `dhcp-range` 並顯示在畫面上，徹底移除 Hardcode。
  * **頻段辨識 (802.11ah)** (`ap_config_editor.go`, `hotspot.go`)：針對 `ieee80211ah=1` 加入了判斷，可以在設定與執行畫面清楚顯示 `Sub-GHz (802.11ah HaLow)`。

## 5. 背景程序殘留清理
* **修改** (`internal/wifi/hotspot.go`, `internal/wifi/connector_disconnect.go`)：
  * 當結束 AP (`Stop()`) 或 Station 斷線時，除了標準的 `wpa_supplicant` 和 `hostapd` 外，加入針對 `_s1g` 版本的程序清理動作 (`pkill -9 wpa_supplicant_s1g`, `killall -9 hostapd_s1g`)，確保在多模式頻繁切換時系統記憶體不會洩漏，且不會有多重服務搶佔 `wlan0` 的問題。

## 6. 驅動前置模組參數與 Channelization Scheme 致命衝突修復
* **問題**：
  * 啟動 AP 模式（選單 `[5]`）時，`hostapd_s1g` 崩潰退出，日誌報錯：
    ```text
    morse: channelization scheme 3 used on ifname wlan0
    Driver channelization scheme (3) does not include configured Op class (70)
    Interface initialization failed
    ```
  * **衝突根源**：
    1. 前置驅動 `dot11ah.ko` 擁有模組參數 `channelization_scheme`（專用於 AU 法規頻段方案切換）。未指定時內核預設為 `3` (IEEE802.11-REVmf / US 方案)。
    2. 主驅動 `morse.ko` 原載入參數硬編碼為 `country=US bcf=bcf_default.bin`。
    3. 但 AP 配置檔 (`default_modules.xml`) 卻指定為 `country_code=AU` 與 `op_class=70`（AU 8MHz 規範要求 Scheme `1`，即 IEEE802.11-2020）。
    4. `hostapd_s1g` 啟動時透過 Netlink 調用 `morse_cli get channelization` 查詢驅動，發現驅動 Scheme (3) 與設定檔要求的 Scheme (1) 嚴重衝突，直接中止初始化。
* **修改架構**：
  1. **模型擴充** (`internal/models/module.go`)：
     在 `KernelConfig` 中增加 `PreloadInsmodArgs` 欄位與 `GetPreloadArgsList()` 方法，支援為前置驅動模組傳遞自訂 insmod 參數。
  2. **驅動載入器** (`internal/wifi/driver_loader.go`)：
     在載入 `PreloadKo`（如 `dot11ah.ko`）時，將對應的 `PreloadInsmodArgs`（`channelization_scheme=1`）注入 `findAndInsmod`。
  3. **模組配置更新** (`assets/default_modules.xml` 與 `internal/models/default_modules.xml`)：
     ```xml
     <Kernel>
         <KoName>morse.ko</KoName>
         <PreloadKo>dot11ah.ko</PreloadKo>
         <PreloadInsmodArgs>channelization_scheme=1</PreloadInsmodArgs>
         <InsmodArgs>country=AU bcf=bcf_default.bin</InsmodArgs>
         <Interface>wlan0</Interface>
         <DriverBackend>nl80211</DriverBackend>
     </Kernel>
     ```
  4. **AP 啟動自動校驗防呆** (`internal/wifi/hotspot.go`)：
     在 `Hotspot.startServices()` 啟動前加入 `ensureMorseChannelizationScheme()`。讀取 `/sys/module/dot11ah/parameters/channelization_scheme`，若當前驅動與 AP 設定檔的國家碼不符（AU 需 scheme 1，US 需 scheme 3），工具自動卸載並以正確 scheme 重新載入，免除使用者手動排查與重載驅動的負擔。

## 7. AP 配置生成器 `op_class` 輸出保留
* **問題**：
  * 在先前的版本中，`ap_config_editor.go` 的 `skipKeys` 與 `managedKeys` 無條件過濾掉了 `"op_class": true`，導致 `default_modules.xml` 中定義的 `<Param key="op_class">70</Param>` 被忽略，`hostapd.conf` 缺漏 `op_class`，造成啟動報錯：
    ```text
    morse_ap_configure_channelization: invalid op_class=0 for country AU
    ```
* **修改** (`internal/config/ap_config_editor.go`)：
  * 將 `op_class` 的略過限制嚴格收斂至 6GHz（6GHz 由程式動態計算 131–137 輸出）。非 6GHz（包含 Sub-GHz HaLow）若定義於 `FixedParams`，完整保留輸出至 `/etc/hostapd.conf`。

## 8. Sub-GHz `morse_cli` 真實頻率與硬體統計解析
* **問題**：
  * Linux 標準 nl80211 核心與 `iw` 不原生支援 Sub-GHz (900MHz) 頻率顯示，導致終端顯示虛擬映射頻率（如 5805 MHz / 5GHz 假象）或速率缺失。
* **修改** (`internal/wifi/connector_helpers.go`)：
  * 當偵測到模組為 `quectel_fgh200m` 時，改由調用 `/usr/sbin/morse_cli -i wlan0 channel` 與 `/usr/sbin/morse_cli -i wlan0 stats`。
  * 精確擷取硬體真實中心頻率（如 `920500 kHz / 920.5 MHz`）、操作頻寬（`8MHz`）、主頻道索引，以及真實收發 MCS 與 PHY 速率，呈現最準確的連線狀態。

