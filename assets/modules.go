package assets

import _ "embed"

// DefaultModulesXML 內嵌全域預設 WiFi 模組設定檔 (Single Source of Truth)
//
//go:embed default_modules.xml
var DefaultModulesXML []byte
