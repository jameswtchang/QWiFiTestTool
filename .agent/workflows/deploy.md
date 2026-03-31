---
description: 編譯並推送 wifi-tool 到開發板
---

// turbo-all

1. 同步 XML 到 internal/models:

```
Copy-Item assets\default_modules.xml internal\models\default_modules.xml -Force
```

1. 交叉編譯 ARM64:

```
$env:CGO_ENABLED=0; $env:GOOS='linux'; $env:GOARCH='arm64'; go build -o wifi-tool cmd/main.go
```

1. 推送到開發板:

```
adb push wifi-tool /data/local/tmp/wifi-tool/wifi-tool
```

1. 推送 XML 配置到開發板:

```
adb push assets\default_modules.xml /data/local/tmp/default_modules.xml
```

1. 設定執行權限:

```
adb shell chmod +x /data/local/tmp/wifi-tool/wifi-tool
```
