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
