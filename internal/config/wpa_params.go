package config

import (
	"fmt"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/ui"
)

// WpaParam represents a wpa_supplicant configuration parameter
type WpaParam struct {
	Key             string
	DescriptionKey  string
	SuggestedKeys   []string // i18n keys for suggested values
	DefaultValue    string
	Example         string
}

// Description returns the translated description
func (p WpaParam) Description() string {
	return i18n.T(p.DescriptionKey)
}

// SuggestedValues returns translated suggested values
func (p WpaParam) SuggestedValues() []string {
	result := make([]string, len(p.SuggestedKeys))
	for i, k := range p.SuggestedKeys {
		result[i] = i18n.T(k)
	}
	return result
}

// GetWpaParams returns all supported wpa_supplicant parameters with descriptions
func GetWpaParams() []WpaParam {
	return []WpaParam{
		{
			Key:            "update_config",
			DescriptionKey: "wpa.update_config.desc",
			SuggestedKeys:  []string{"wpa.update_config.s0", "wpa.update_config.s1"},
			DefaultValue:   "1",
			Example:        "update_config=1",
		},
		{
			Key:            "ap_scan",
			DescriptionKey: "wpa.ap_scan.desc",
			SuggestedKeys:  []string{"0", "1", "2"},
			DefaultValue:   "1",
			Example:        "ap_scan=1",
		},
		{
			Key:            "eapol_version",
			DescriptionKey: "wpa.eapol_version.desc",
			SuggestedKeys:  []string{"wpa.eapol_version.s1", "wpa.eapol_version.s2"},
			DefaultValue:   "1",
			Example:        "eapol_version=1",
		},
		{
			Key:            "fast_reauth",
			DescriptionKey: "wpa.fast_reauth.desc",
			SuggestedKeys:  []string{"wpa.update_config.s0", "wpa.update_config.s1"},
			DefaultValue:   "1",
			Example:        "fast_reauth=1",
		},
		{
			Key:            "pmf",
			DescriptionKey: "wpa.pmf.desc",
			SuggestedKeys:  []string{"wpa.pmf.s0", "wpa.pmf.s1", "wpa.pmf.s2"},
			DefaultValue:   "1",
			Example:        "pmf=1",
		},
		{
			Key:            "wowlan_triggers",
			DescriptionKey: "wpa.wowlan.desc",
			SuggestedKeys:  []string{"any", "disconnect", "magic_pkt", "gtk_rekey_failure"},
			DefaultValue:   "any",
			Example:        "wowlan_triggers=any",
		},
		{
			Key:            "p2p_add_cli_chan",
			DescriptionKey: "wpa.p2p_add.desc",
			SuggestedKeys:  []string{"wpa.update_config.s0", "wpa.update_config.s1"},
			DefaultValue:   "0",
			Example:        "p2p_add_cli_chan=0",
		},
		{
			Key:            "sae_pwe",
			DescriptionKey: "wpa.sae_pwe.desc",
			SuggestedKeys:  []string{"0 (hunting-and-pecking)", "1 (hash-to-element)", "wpa.sae_pwe.s2"},
			DefaultValue:   "2",
			Example:        "sae_pwe=2",
		},
		{
			Key:            "p2p_disabled",
			DescriptionKey: "wpa.p2p_disabled.desc",
			SuggestedKeys:  []string{"wpa.p2p_disabled.s0", "wpa.p2p_disabled.s1"},
			DefaultValue:   "1",
			Example:        "p2p_disabled=1",
		},
		{
			Key:            "country",
			DescriptionKey: "wpa.country.desc",
			SuggestedKeys:  []string{"US", "CN", "TW", "JP", "EU"},
			DefaultValue:   "US",
			Example:        "country=US",
		},
	}
}

// PromptParamEdit prompts user to edit a specific parameter
func PromptParamEdit(param WpaParam) (string, bool) {
	fmt.Printf("\n"+ui.Cyan+ui.Bold+"=== %s ==="+ui.Reset+"\n", param.Key)
	fmt.Printf(ui.Bold+i18n.T("wpa.param.desc_label")+ui.White+"%s"+ui.Reset+"\n", param.Description())
	fmt.Printf(ui.Bold+i18n.T("wpa.param.suggest")+ui.Green+"%v"+ui.Reset+"\n", param.SuggestedValues())
	fmt.Printf(ui.Bold+i18n.T("wpa.param.default")+ui.Cyan+"%s"+ui.Reset+"\n", param.DefaultValue)
	fmt.Printf(ui.Bold+i18n.T("wpa.param.example")+ui.White+"%s"+ui.Reset+"\n", param.Example)

	fmt.Print("\n" + ui.Yellow + ui.Bold + i18n.T("wpa.param.confirm") + ui.Reset)
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		return "", false
	}

	fmt.Printf(ui.Yellow+ui.Bold+i18n.Tf("wpa.param.input", param.DefaultValue)+ui.Reset)
	var value string
	fmt.Scanln(&value)

	if value == "" {
		value = param.DefaultValue
	}

	return value, true
}

