---
marp: true
theme: default
paginate: true
header: 'QWiFiTestTool Project Proposal'
footer: '2026-02-06 | Internal Review'
backgroundColor: #ffffff
---

<!-- _class: lead -->
# QWiFiTestTool 專案分析與提案

## 打造跨平台、智慧化的 WiFi 開發測試利器

---

# 1. 專案背景與痛點 (Problem Statement)

在嵌入式 Linux 與 Android 平台的 WiFi 開發過程中，我們面臨以下挑戰：

- **繁瑣的手動配置**：每次測試需手動編輯 `wpa_supplicant.conf` 或 `hostapd.conf`，極易出錯。
- **環境差異**：不同平台（Linux PC, Buildroot, Android）的指令與路徑不一致（如 `ifconfig` vs `ip`, `dhclient` vs `udhcpc`）。
- **除錯困難**：當連接失敗時，缺乏整合性的診斷資訊，Logs 分散在不同地方（dmesg, system logs）。
- **模組多樣性**：不同 WiFi 晶片（Realtek, Qualcomm, MTK）亦有不同的參數需求。

---

# 2. 解決方案：QWiFiTestTool (Solution)

**QWiFiTestTool** 是一個用 **Go** 語言開發的輕量級、跨平台 WiFi 測試與配置工具。

### 核心價值

1. **Write Once, Run Anywhere**: 單一執行檔，同時支援 Linux (x86/ARM) 與 Android。
2. **自動化配置**: 根據 XML 定義檔，自動生成標準化的 WiFi 配置檔。
3. **互動式操作**: 提供直覺的 CLI 選單，無需記憶複雜參數。
4. **增強型除錯**: 自動收集發生問題當下的關鍵 Log 與狀態。

---

# 3. 系統架構 (Architecture)

```mermaid
graph TD
    User[使用者] --> UI[CLI 互動介面]
    UI --> Core[核心邏輯層]
    
    subgraph "Core Functionality"
        Scanner[掃描器]
        Station[Station 連接]
        AP[AP 熱點]
        Diag[診斷模組]
    end
    
    Core --> Scanner
    Core --> Station
    Core --> AP
    Core --> Diag
    
    subgraph "Abstraction Layer"
        ConfigMgr[配置生成器]
        SysExec[系統命令適配器]
    end
    
    Scanner --> SysExec
    Station --> ConfigMgr
    AP --> ConfigMgr
    
    ConfigMgr --> Files[配置檔 (.conf)]
    SysExec --> Tools[系統工具 (wpa_cli, ip, hostapd)]
```

---

# 4. 關鍵功能介紹 (Key Features)

### 📡 Station 模式 (Client)

- **智慧掃描**：清晰展示 SSID、信號強度、安全性。
- **自動重連**：支援失敗後自動切換至手動掃描模式。
- **錯誤引導**：當連接失敗（如密碼錯誤），提供原因分析建議。

### 🚀 AP 模式 (Hotspot)

- **完整服務**：整合 `hostapd` (無線接入) + `dnsmasq` (DHCP/DNS)。
- **日誌留存**：啟動與停止時自動保存 `dmesg` 與 `hostapd` 詳細日誌，便利事後分析。
- **即時狀態**：啟動後顯示詳細的 AP 資訊（IP 範圍、加密協議、頻段）。

---

# 5. 近期優化亮點 (Recent Improvements)

針對開發與測試需求，我們近期完成了以下重大更新：

1. **WiFi 協議識別**：
   - 掃描列表可精準顯示 **WiFi 4 / 5 / 6 / 6E / 7**。
   - 解析 Flags 與頻段資訊，不再只顯示 generic 的加密方式。

2. **強化的 AP 測試模式**：
   - 解決了僅有 hostapd 無法連網的問題，加入 dnsmasq 支援。
   - 優化日誌輸出：所有執行命令與設定檔內容皆會打印，資訊透明化。
   - 啟動與停止時自動捕捉 Kernel Log (`dmesg`)。

3. **使用者體驗 (UX)**：
   - 優化列表排版，過長 SSID 自動截斷。
   - 異常流程處理：當 `wpa_supplicant` 啟動失敗，主動引導進入掃描模式。

---

# 6. 技術優勢 (Technical Advantages)

- **Go 語言開發**：
  - **靜態編譯**：編譯出的 Binary 不依賴 libc 版本，極易部署到各種 Embedded 系統。
  - **高併發**：利用 Goroutine 處理掃描與日誌監控，介面不卡頓。

- **高度可配置 (XML Based)**：
  - 透過 `default_modules.xml` 定義不同模組（如 Realtek 8733BS）的驅動參數。
  - 新增支援模組無需修改程式碼，僅需更新 XML。

---

# 7. 未來展望 (Roadmap)

1. **自動化測試腳本支援**：
   - 支援 `--script` 參數，允許免互動執行長時間壓力測試（如反覆開關 WiFi）。

2. **Web UI 介面**：
   - 內建小型 Web Server，允許透過瀏覽器遠端配置與查看狀態（適用於無螢幕設備）。

3. **效能分析工具**：
   - 整合 `iperf3` 自動測試並生成吞吐量報告。

---

<!-- _class: invert -->
# Q & A

## 感謝您的聆聽
