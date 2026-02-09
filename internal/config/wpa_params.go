package config

import "fmt"

// WpaParam represents a wpa_supplicant configuration parameter
type WpaParam struct {
	Key             string
	Description     string
	SuggestedValues []string
	DefaultValue    string
	Example         string
}

// GetWpaParams returns all supported wpa_supplicant parameters with descriptions
func GetWpaParams() []WpaParam {
	return []WpaParam{
		{
			Key:             "update_config",
			Description:     "允許 wpa_cli/wpa_gui 動態更新配置檔案",
			SuggestedValues: []string{"0 (禁用)", "1 (啟用)"},
			DefaultValue:    "1",
			Example:         "update_config=1",
		},
		{
			Key:             "ap_scan",
			Description:     "AP 掃描模式 (0=由驅動控制, 1=由wpa_supplicant控制, 2=混合模式)",
			SuggestedValues: []string{"0", "1", "2"},
			DefaultValue:    "1",
			Example:         "ap_scan=1",
		},
		{
			Key:             "eapol_version",
			Description:     "IEEE 802.1X/EAPOL 協議版本",
			SuggestedValues: []string{"1 (IEEE 802.1X-2001)", "2 (IEEE 802.1X-2004)"},
			DefaultValue:    "1",
			Example:         "eapol_version=1",
		},
		{
			Key:             "fast_reauth",
			Description:     "EAP 快速重新認證 (僅用於企業網路)",
			SuggestedValues: []string{"0 (禁用)", "1 (啟用)"},
			DefaultValue:    "1",
			Example:         "fast_reauth=1",
		},
		{
			Key:             "pmf",
			Description:     "受保護管理框架 (Protected Management Frames) - 802.11w",
			SuggestedValues: []string{"0 (禁用)", "1 (可選)", "2 (必需)"},
			DefaultValue:    "1",
			Example:         "pmf=1",
		},
		{
			Key:             "wowlan_triggers",
			Description:     "Wake-on-WLAN 觸發條件 (斷線時喚醒)",
			SuggestedValues: []string{"any", "disconnect", "magic_pkt", "gtk_rekey_failure"},
			DefaultValue:    "any",
			Example:         "wowlan_triggers=any",
		},
		{
			Key:             "p2p_add_cli_chan",
			Description:     "P2P 客戶端通道添加優先級",
			SuggestedValues: []string{"0 (禁用)", "1 (啟用)"},
			DefaultValue:    "0",
			Example:         "p2p_add_cli_chan=0",
		},
		{
			Key:             "sae_pwe",
			Description:     "SAE 密碼元素生成方法 (WPA3安全性)",
			SuggestedValues: []string{"0 (hunting-and-pecking)", "1 (hash-to-element)", "2 (兩者)"},
			DefaultValue:    "2",
			Example:         "sae_pwe=2",
		},
		{
			Key:             "p2p_disabled",
			Description:     "禁用 Wi-Fi Direct (P2P) 功能",
			SuggestedValues: []string{"0 (啟用P2P)", "1 (禁用P2P)"},
			DefaultValue:    "1",
			Example:         "p2p_disabled=1",
		},
		{
			Key:             "country",
			Description:     "國家/地區代碼 (影響頻道和功率限制)",
			SuggestedValues: []string{"US", "CN", "TW", "JP", "EU"},
			DefaultValue:    "US",
			Example:         "country=US",
		},
	}
}

// PromptParamEdit prompts user to edit a specific parameter
func PromptParamEdit(param WpaParam) (string, bool) {
	fmt.Printf("\n=== %s ===\n", param.Key)
	fmt.Printf("說明: %s\n", param.Description)
	fmt.Printf("建議值: %v\n", param.SuggestedValues)
	fmt.Printf("預設值: %s\n", param.DefaultValue)
	fmt.Printf("範例: %s\n", param.Example)

	fmt.Print("\n是否配置此參數? [y/N]: ")
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		return "", false
	}

	fmt.Printf("請輸入值 (直接按 Enter 使用預設值 '%s'): ", param.DefaultValue)
	var value string
	fmt.Scanln(&value)

	if value == "" {
		value = param.DefaultValue
	}

	return value, true
}
