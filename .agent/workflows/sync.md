---
description: 同步修改到 GitHub
---

# Git Sync Workflow

此 workflow 會自動檢查當前修改，生成 commit message，並推送到 GitHub。

## 步驟

1. **檢查修改狀態**

   ```bash
   $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
   git status
   ```

2. **加入所有修改**

   ```bash
   git add .
   ```

3. **生成 Commit Message**
   - 使用 `git diff --cached` 檢查修改內容
   - 根據修改內容生成**有意義且詳細的** commit message。不只要寫出做了什麼，也要簡述原因或關鍵變更。
   - 格式：`<類型>: <簡短描述>`
   - 類型範例：
     - `feat`: 新功能
     - `fix`: 錯誤修復
     - `refactor`: 重構
     - `docs`: 文件更新
     - `style`: 格式調整
     - `perf`: 性能優化

4. **提交並推送**

   ```bash
   git commit -m "<生成的 commit message>"
   git push
   ```

## 注意事項

- 確保 Git 環境變數已正確設定
- 推送前會顯示修改摘要供確認
- 如有衝突需手動解決
