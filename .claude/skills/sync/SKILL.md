---
name: sync
description: 檢查修改、版本號檢查與遞增、MD 文檔同步、本機與實機驗證、生成 Conventional Commit 並推送至 GitHub
disable-model-invocation: true
argument-hint: "[--quick | --skip-hw]"
---

# Sync：版本號 → 文檔同步 → 本機驗證 → 實機驗證 → Commit → Push

工作目錄：`D:\QWiFiTestTool\QWifiTestTool`（以下指令皆用 PowerShell 執行）。

這是 AGENTS.md 中唯一允許 commit / push 的流程。任一步驟失敗即停止並回報，**不得在驗證未通過時提交**。

本次參數：`$ARGUMENTS`

| 參數 | 實機驗證範圍 |
|:---|:---|
| *(無)* | 完整分層 E2E：AP 矩陣 29 + STA 策略 11 + 核心 RF 6（約 10 分鐘） |
| `--quick` | 僅靜態矩陣 `-MatrixOnly`（約 1 分鐘，不發射射頻） |
| `--skip-hw` | 略過實機驗證；commit message 必須註明「未實機驗證」 |

---

## 1. 檢查並分類修改

```powershell
git status -s
git diff
git branch --show-current
```

- 若無任何變更，回報「工作目錄乾淨，無需同步」並結束。
- 判斷類型：`feat` / `fix` / `refactor` / `perf` / `test` / `docs` / `chore`，釐清修改背景、技術決策與受影響模組。
- 將修改分為兩類，後續步驟依此決定是否遞增版本與是否需要實機驗證：
  - **程式修改**：`cmd/`、`internal/`（`*_test.go` 除外）、`assets/`、`scripts/*.ps1`、`scripts/*.sh`、`go.mod`
  - **非程式修改**：`*.md`、`*_test.go`、`.claude/`、`.agent/`
- 對每個 `M` 檔執行 `git diff --quiet -- <file>`：無內容差異者（僅時間戳 / 換行格式）不視為修改，於步驟 6 只刷新索引、不提交。

## 2. 版本號檢查與修改

版本號唯一來源：`cmd/main.go` 的 `const Version = "V<major>.<minor>"`（banner 自動引用）。

```powershell
$cur  = (Select-String -Path cmd\main.go -Pattern 'const Version = "(V[\d.]+)"').Matches[0].Groups[1].Value
$head = ((git show HEAD:cmd/main.go) | Select-String -Pattern 'const Version = "(V[\d.]+)"').Matches[0].Groups[1].Value
"HEAD: $head / 工作目錄: $cur"
```

判斷規則：

| 情況 | 處理 |
|:---|:---|
| 有程式修改，且 `$cur == $head` | 最後一段 +1（例：`V1.16` → `V1.17`），修改 `cmd/main.go` |
| 有程式修改，且 `$cur` 恰為 `$head` +1 | 已遞增，維持不變 |
| `$cur` 跳號、倒退或格式不符 | 停止，向使用者確認正確版本號 |
| 僅非程式修改 | 不遞增；若 `$cur != $head` 依上列規則檢查 |

記下最終版本號 `V<x>`，供步驟 3 文檔與步驟 7 commit 標題使用。

## 3. MD 文檔更新

原則：
- 文檔描述必須與程式實際行為一致；發現既有的錯誤或過時描述一併修正。
- 未經實機驗證的功能必須標註「（尚未實機驗證）」。
- 維持既有風格：繁體中文、既有章節編號與格式；歷史章節除事實錯誤外不改寫。
- **驗證數字**（PASS 數量等）一律等步驟 5 實際結果出來後再填入，不可預填。

### 3.1 依修改範圍對照更新

| 修改範圍 | 必須更新的文檔 |
|:---|:---|
| 版本號遞增 | `README.md` 底部 `**版本**` 與 `Updated` 日期；`docs/project_summary.md` 新增章節 `## N. 最新更新 (YYYY-MM-DD) — V<x> (摘要)`，含各子章節與「驗證成果」 |
| 新功能或行為改變 | `README.md`「功能特性」新增 `✅ **名稱 (V<x>)**: 說明`；選單或流程改變時更新「使用方式」各段 |
| 新增 / 刪除檔案或目錄 | `README.md`「目錄結構」 |
| `assets/default_modules.xml` 模組增刪或晶片 / 驅動變更 | `README.md`「支援模組」表、`internal/sys/README.md`、`assets/android_arm64/README.md` 的對照表，模組數量必須與 XML 的 `<Module>` 數一致 |
| 特定模組行為（FGH200M、FCS945R 等） | `docs/FGH200M_SUMMARY.md`、`docs/FCS945R_SUMMARY.md` 等對應文檔 |
| `scripts/` | `docs/e2e_wifi_test_guide.md`、`scripts/README.md`（參數表、前置條件、新版本段落） |
| `.claude/skills/` 或 `.agent/workflows/` | 另一側保持一致；工作流規則改變時同步 `AGENTS.md` 與 `README.md` 目錄結構 |
| 架構調整 | `docs/presentation_outline.md` |

### 3.2 一致性檢查

```powershell
Select-String -Path README.md -Pattern '\*\*版本\*\*'
Select-String -Path docs\project_summary.md -Pattern '^## \d+\. 最新更新' | Select-Object -Last 1
"XML 模組數: "      + ([xml](Get-Content -Raw -Encoding UTF8 assets\default_modules.xml)).WiFiTool.Module.Count
"README 模組表列數: " + (Select-String -Path README.md -Pattern '^\| \d+ \| Quectel').Count
git diff --stat -- '*.md'
```

- README 版本行、`project_summary.md` 最新章節標題都必須是 `V<x>`。
- 支援模組表筆數與 XML 模組數一致。
- 列出本次更新的文檔清單，供最後回報。

## 4. 本機品質驗證

在主機原生平台執行（不可設定 `GOOS` / `GOARCH`）：

```powershell
go vet ./...
go test -count=1 ./...
```

失敗或有 vet 警告即停止。

## 5. 實機驗證（有程式修改時必做）

僅非程式修改，或參數為 `--skip-hw` 時略過本步驟（後者須在 commit message 註明）。

### 5.1 確認開發板

```powershell
adb devices
adb shell pidof QWifiTestTool
```

- 必須恰好一台裝置狀態為 `device`。無裝置時停止，請使用者接上開發板，或改用 `/sync --skip-hw`。
- 若 `pidof` 有輸出，代表板上有人正在使用工具。E2E 會終止它並重設 `wlan0`，必須先詢問使用者。

### 5.2 部署最新版本

依 `.claude/skills/deploy/SKILL.md` 的步驟 2～5 編譯 ARM64 版並推送，確認板端執行檔大小與本機一致。AGENTS.md 允許 `/sync` 在此步驟部署。

### 5.3 執行 E2E

```powershell
# 無參數：完整分層測試；--quick：加上 -MatrixOnly
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e_wifi_test.ps1 [-MatrixOnly] *> "$env:TEMP\qwt_sync_e2e.log"
```

- 完整測試約 10 分鐘，以背景方式執行，完成後讀取 log 中的報告表與 `Summary`。
- **任何 FAIL 即停止，不得提交**：依 `.claude/skills/verify-hardware/SKILL.md` 步驟 4 收集 dmesg 與 hostapd 日誌並回報。
- 記下實際結果（各 Phase 的 PASS 數），回填步驟 3 的「驗證成果」並用於 commit message。
- 結束後刪除本機 log 與板端測試殘留。

注意：從 PowerShell 傳給 `adb shell` 的參數若含雙引號、`|`、`(`，PowerShell 5.1 可能拿掉引號，導致板端 shell 誤執行。複雜的板端指令請先寫成腳本檔，`adb push` 後再用 `adb shell sh <檔案>` 執行。

## 6. 明確加入暫存區（禁止 `git add .`）

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

步驟 1 判定為「僅時間戳 / 換行格式」的檔案：執行 `git add -- <file>` 刷新索引後，確認 `git diff --cached -- <file>` 無輸出。

## 7. 生成 Commit Message

遵循 Conventional Commits；有版本遞增時標題帶版本號：

```text
<type>(v<x>): <精確簡短的摘要>

- <變更點 1：修改原因與技術細節>
- <變更點 2：修復的邊界情況或架構優化>
- 驗證：<本機測試結果>；<實機 E2E 實際結果，或「未實機驗證 (--skip-hw)」>
- 文檔：<本次更新的 MD 文檔清單>

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```

將訊息寫入暫存檔（scratchpad，UTF-8 無 BOM）再用 `-F` 提交，避免 PowerShell 多行引號與跳脫問題：

```powershell
git commit -F <commit_msg 暫存檔>
```

提交後刪除暫存檔。

## 8. 推送至 GitHub

```powershell
git push origin HEAD
git status -sb
git log -1 --oneline
```

- PowerShell 5.1 會把 git 寫到 stderr 的進度訊息顯示成錯誤，以 `$LASTEXITCODE` 與 `git status -sb` 判斷是否成功。
- 推送被拒（例如遠端有新 commit）時停止並回報，不要 force push。

## 回報

- commit hash、分支名稱、推送範圍（`舊..新`）
- 版本號變化（`V舊 → V新`，或「未變更」及原因）
- 已更新的 MD 文檔清單
- 驗證結果：本機測試、實機 E2E 各 Phase 的 PASS 數（或略過原因）
- 確認工作目錄乾淨（`## main...origin/main`）
