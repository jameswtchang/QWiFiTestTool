---
description: 自動檢查修改、更新 MD 文件、生成 commit message 並推送至 GitHub
---

# Git Sync Workflow

此 workflow 會自動檢查當前修改、更新相關 Markdown 文檔、驗證程式碼品質、生成結構化 Commit Message，並推送到 GitHub。

工作目錄: `d:\QWiFiTestTool\QWifiTestTool`

---

## 執行流程

### 1. 檢查當前修改 (Inspect Working Tree)
- 檢查環境變數並獲取變更狀態：
  ```powershell
  $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
  git status -s
  git diff
  ```
- 仔細分析所有已變更的程式碼與配置：
  - 識別是否有新增功能 (`feat`)、錯誤修正 (`fix`)、架構重構 (`refactor`)、效能優化 (`perf`) 或測試調整 (`test`)。
  - 釐清修改的背景、關鍵技術決策與受影響的模組範圍。

### 2. 審查並同步更新 Markdown 文檔 (Update Markdown Docs)
- **原則**：程式碼與文檔必須維持 100% 同步，嚴禁程式碼更新而文檔滯後。
- 根據步驟 1 發現的變更，主動檢查並更新對應的 MD 文檔：
  1. **`README.md`**：
     - 若有版本更新（如 `cmd/main.go` 遞增版本），同步更新底部版本號與日期。
     - 若有新功能特性、新支援模組或 CLI/TUI 選單變更，更新「功能特性」、「使用方式」及「支援模組」表格。
  2. **`docs/project_summary.md`**：
     - 在最新章節或最新版本段落中，增補本次變更的設計背景、架構演進、修復機制與驗證成果。
  3. **特定專案文檔**（視變更範圍同步更新）：
     - E2E 測試相關 → `docs/e2e_wifi_test_guide.md`、`scripts/README.md`
     - 架構與簡報大綱 → `docs/presentation_outline.md`
     - 模組配置或硬體相關 → `docs/FGH200M_SUMMARY.md`、`docs/FCS945R_SUMMARY.md` 等
- 更新完成後，使用 `git status -s` 確認文檔變更已就緒。

### 3. 品質與測試驗證 (Verification)
- 確保所有修改未引入 regression：
  ```powershell
  go test -count=1 ./...
  go vet ./...
  ```
- 若測試失敗或有語法警告，優先修復後再進入下一步。

### 4. 加入暫存區與生成結構化 Commit Message (Stage & Commit Message)
- 將所有變更（含更新的 MD 文檔）加入暫存區：
  ```powershell
  git add .
  git diff --cached --stat
  ```
- 根據暫存區實際內容，依循 **Conventional Commits** 規範生成高水準 Commit Message：
  - **格式**：
    ```text
    <type>(<scope>): <精確簡短的中文或英文摘要>

    - <詳細變更點 1：包含修改原因與技術細節>
    - <詳細變更點 2：修復的特定邊界情況或架構優化>
    - <文檔與測試同步說明>
    ```
  - **Type 清單**：
    - `feat`: 新增功能特性（如 AP 預設、STA 策略）
    - `fix`: 錯誤修正（如 DFS 依賴修正、超時處理）
    - `refactor`: 重構（如 UI 分組輸出、函數封裝）
    - `docs`: 文檔更新
    - `test`: 測試腳本或單元測試
    - `perf`: 效能優化
    - `chore`: 建置或維護性調整

### 5. 提交並推送至 GitHub (Commit & Push)
- 執行提交與遠端推送：
  ```powershell
  git commit -m "<生成的 Commit Message>"
  git push origin main
  ```
- 檢查推送後狀態：
  ```powershell
  git status
  ```
- 確認工作目錄為乾淨（clean）狀態，回報 Commit Hash、分支名稱與本次同步摘要。
