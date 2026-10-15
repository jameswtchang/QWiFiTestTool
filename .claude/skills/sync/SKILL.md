---
name: sync
description: 檢查修改、同步更新 MD 文件、驗證測試、生成 Conventional Commit 並推送至 GitHub
disable-model-invocation: true
---

# Sync：文檔同步 → 驗證 → Commit → Push

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

這是 AGENTS.md 中唯一允許 commit / push 的流程。任一步驟失敗即停止並回報。

## 1. 檢查當前修改

```powershell
git status -s
git diff
```

分析所有變更：

- 判斷類型：`feat` / `fix` / `refactor` / `perf` / `test` / `docs` / `chore`。
- 釐清修改背景、關鍵技術決策與受影響模組。
- 若無任何變更，回報「工作目錄乾淨，無需同步」並結束。

## 2. 同步更新 Markdown 文檔

原則：程式碼與文檔必須同步，不可程式碼更新而文檔滯後。依步驟 1 的變更檢查並更新：

1. **`README.md`**
   - `cmd/main.go` 的 `Version` 有變更時，同步更新底部版本號與日期。
   - 新功能、新支援模組或 TUI 選單變更時，更新「功能特性」、「使用方式」、「支援模組」表格。
2. **`docs/project_summary.md`**：於最新版本段落增補本次變更的設計背景、修復機制與驗證成果。
3. **特定文檔**（視變更範圍）：
   - E2E 測試 → `docs/e2e_wifi_test_guide.md`、`scripts/README.md`
   - 架構與簡報 → `docs/presentation_outline.md`
   - 模組配置或硬體 → `docs/FGH200M_SUMMARY.md`、`docs/FCS945R_SUMMARY.md` 等
   - 工作流 / skill 變更 → `.claude/skills/` 與 `.agent/workflows/` 兩處保持一致

## 3. 品質驗證

```powershell
go vet ./...
go test -count=1 ./...
```

測試失敗或有 vet 警告時，停止並回報，不可提交。

## 4. 明確加入暫存區（禁止 `git add .`）

逐一列出要提交的檔案並明確加入：

```powershell
git add -- <file1> <file2> ...
git diff --cached --stat
```

**不得加入**下列檔案；若出現在 `git status` 中，列出並詢問使用者：

- 編譯產物：`*.exe`、`QWifiTestTool*`、`qwifitesttool*`、`out/`、`bin/`
- 日誌與暫存：`*.log`、`_scratch/`、`*.diff`、`*.zip`
- 疑似機密：含金鑰、密碼、token 的檔案
- 根目錄重複檔：`country_channel_lookup.json`（正本在 `internal/models/`）

## 5. 生成 Commit Message

依暫存區實際內容，遵循 Conventional Commits：

```text
<type>(<scope>): <精確簡短的摘要>

- <變更點 1：修改原因與技術細節>
- <變更點 2：修復的邊界情況或架構優化>
- <文檔與測試同步說明>

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```

將訊息寫入暫存檔再用 `-F` 提交，避免 PowerShell 多行引號與跳脫問題：

```powershell
$msgFile = Join-Path $env:TEMP 'qwt_commit_msg.txt'
# 以 Write 工具將 commit message 寫入 $msgFile (UTF-8, 無 BOM)
git commit -F $msgFile
Remove-Item $msgFile
```

## 6. 推送至 GitHub

```powershell
git branch --show-current
git push origin HEAD
git status
git log -1 --oneline
```

- 推送被拒（例如遠端有新 commit）時停止並回報，不要 force push。
- 確認工作目錄乾淨，回報 commit hash、分支名稱、變更摘要與已更新的文檔清單。
