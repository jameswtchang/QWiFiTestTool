# XML 檔案自動同步說明

## 設計原因

QWifiTestTool 有兩個 `default_modules.xml` 檔案：

1. **assets/default_modules.xml** - 主要編輯檔案
2. **internal/models/default_modules.xml** - 嵌入到二進制檔案中

### 為什麼需要兩份？

Due to Go `embed` directive limitations, we cannot use `../` to reference parent directories:

```go
//go:embed default_modules.xml  // ✅ OK - same directory
//go:embed ../../assets/...      // ❌ ERROR - parent directory not allowed
```

---

## 自動同步機制

### 建置時自動同步

**build.bat** (Windows):
```batch
echo [INFO] Syncing XML configuration files...
copy /Y assets\default_modules.xml internal\models\default_modules.xml >nul
```

**build.sh** (Linux/macOS):
```bash
echo "[INFO] Syncing XML configuration files..."
cp assets/default_modules.xml internal/models/default_modules.xml
```

### 使用方式

**編輯 XML**：
```
只編輯 assets/default_modules.xml
```

**建置程式**：
```batch
# Windows
build.bat

# Linux/macOS
./build.sh
```

建置腳本會自動將 `assets/default_modules.xml` 複製到 `internal/models/`，然後編譯。

---

## 工作流程

### 1. 新增或修改模組

編輯 `assets/default_modules.xml`：

```xml
<Module id="new_module">
    <DisplayName>New Module</DisplayName>
    ...
</Module>
```

### 2. 執行建置

```batch
build.bat
```

輸出：
```
[INFO] Syncing XML configuration files...
Source: assets\default_modules.xml
Target: internal\models\default_modules.xml
[SUCCESS] XML files synchronized

[INFO] Building for Windows (testing)...
```

### 3. 完成

- ✅ XML 已自動同步
- ✅ 新配置已嵌入二進制檔案
- ✅ 無需手動複製

---

## 手動同步（如需要）

### Windows (PowerShell)
```powershell
Copy-Item -Path assets\default_modules.xml -Destination internal\models\default_modules.xml -Force
```

### Windows (CMD)
```cmd
copy /Y assets\default_modules.xml internal\models\default_modules.xml
```

### Linux/macOS
```bash
cp assets/default_modules.xml internal/models/default_modules.xml
```

---

## 檔案角色

| 檔案 | 角色 | 編輯 |
|------|------|------|
| `assets/default_modules.xml` | 主要來源 | ✅ 編輯此檔案 |
| `internal/models/default_modules.xml` | 嵌入版本 | ❌ 自動生成 |

---

## 注意事項

⚠️ **重要**: 
- 只編輯 `assets/default_modules.xml`
- 使用 `build.bat` 或 `build.sh` 建置
- `internal/models/default_modules.xml` 會自動更新

✅ **好處**:
- 單一真實來源 (`assets/`)
- 自動同步，無需手動操作
- 建置流程簡單明確

---

**版本**: v2.1.1  
**更新日期**: 2026-02-02
