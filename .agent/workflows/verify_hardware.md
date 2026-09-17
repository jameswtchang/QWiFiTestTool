---
description: 執行 QWiFiTestTool 實機硬體閉環端到端功能驗證 (Qualcomm FME170Q)
---

# QWiFiTestTool 實機硬體閉環驗證 Workflow

此 workflow 會透過 ADB 連接開發板，自動化操作 QWifiTestTool 進行 AP 規格廣播發射與 STA 策略連線，並雙重斷言 TUI 終端輸出與 Linux 核心射頻狀態。

## 步驟

1. **確認開發板與 WiFi 模組狀態**

   ```powershell
   adb devices
   adb shell "uname -a; lsmod | grep -E 'wlan|cnss'"
   ```

2. **確保開發板上的 QWifiTestTool 為最新版本**

   若代碼有更新，請先執行 `/deploy` 重新編譯與推送。

3. **執行實機自動化驗測 Agent**

   - 執行完整驗測（AP 規格發射 + STA 策略連線）：
     ```powershell
     python scripts/e2e_wifi_test_agent.py
     ```

   - 僅執行 AP 模式實機發射驗證 (6G 320M, 5G 160M/80M, 2.4G 40M)：
     ```powershell
     python scripts/e2e_wifi_test_agent.py --ap-only
     ```

   - 僅執行 STA 模式連線與協定降級驗證：
     ```powershell
     python scripts/e2e_wifi_test_agent.py --sta-only
     ```

4. **搜集測試過程異常日誌（若有任何測項 FAIL）**

   ```powershell
   adb shell "dmesg | tail -n 60"
   adb shell "cat /tmp/wifi_logs/*.log 2>/dev/null | tail -n 80"
   ```
