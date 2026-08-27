package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
)

// ShowBanner displays the application banner with dynamic width
func ShowBanner() {
	lines := []string{
		i18n.T("banner.title"),
		i18n.T("banner.platform"),
		i18n.T("banner.version"),
		i18n.T("banner.author"),
	}

	// Find the widest line (visual width)
	maxWidth := 0
	for _, line := range lines {
		w := VisualWidth(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Box inner width = max line width + padding (4 spaces each side)
	boxInner := maxWidth + 8
	border := strings.Repeat("═", boxInner)

	fmt.Println(Cyan + Bold + "╔" + border + "╗" + Reset)
	for _, line := range lines {
		pad := boxInner - VisualWidth(line)
		left := pad / 2
		right := pad - left
		fmt.Println(Cyan + Bold + "║" + strings.Repeat(" ", left) + line + strings.Repeat(" ", right) + "║" + Reset)
	}
	fmt.Println(Cyan + Bold + "╚" + border + "╝" + Reset)
	fmt.Println()
}

// ShowModuleMenu displays available modules and prompts for selection
func ShowModuleMenu(config *models.WiFiToolConfig) (*models.WiFiModule, error) {
	fmt.Println(Blue + Bold + "========== " + i18n.T("menu.modules.title") + " ==========" + Reset)

	modules := config.ListModules()
	for _, mod := range modules {
		modStr := strings.Replace(mod, "[", Cyan+Bold+"[", 1)
		modStr = strings.Replace(modStr, "]", "]"+Reset, 1)
		fmt.Println(modStr)
	}

	fmt.Println(Blue + Bold + i18n.T("menu.modules.sep") + Reset)
	fmt.Print(Yellow + Bold + i18n.T("menu.modules.select") + Reset)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var selection int
	fmt.Sscanf(input, "%d", &selection)

	if selection < 1 || selection > len(config.Modules) {
		return nil, fmt.Errorf("invalid selection")
	}

	return &config.Modules[selection-1], nil
}

// ShowOperationMenu displays operation mode menu
func ShowOperationMenu() string {
	fmt.Println("\n" + Yellow + Bold + "========== " + i18n.T("menu.op.title") + " ==========" + Reset)
	fmt.Println(Cyan + Bold + "[1]" + Reset + " " + i18n.T("menu.op.1"))
	fmt.Println(Cyan + Bold + "[2]" + Reset + " " + i18n.T("menu.op.2"))
	fmt.Println(Cyan + Bold + "[3]" + Reset + " " + i18n.T("menu.op.3"))
	fmt.Println(Cyan + Bold + "[4]" + Reset + " " + i18n.T("menu.op.4"))
	fmt.Println(Cyan + Bold + "[5]" + Reset + " " + i18n.T("menu.op.5"))
	fmt.Println(Cyan + Bold + "[0]" + Reset + " " + i18n.T("menu.op.0"))
	fmt.Println(Yellow + Bold + i18n.T("menu.op.sep") + Reset)
	fmt.Print(Yellow + Bold + i18n.T("menu.op.select") + Reset)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// PromptContinue asks user if they want to continue
func PromptContinue() bool {
	fmt.Print("\n" + Yellow + Bold + i18n.T("prompt.continue") + Reset)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	return input == "y" || input == "yes"
}

// ShowError displays an error message
func ShowError(err error) {
	fmt.Printf("\n"+Red+Bold+i18n.T("err.prefix")+Reset+"\n\n", err)
}

// ShowSuccess displays a success message
func ShowSuccess(msg string) {
	fmt.Printf("\n"+Green+Bold+"✓ %s"+Reset+"\n\n", msg)
}

// VisualWidth calculates the visual display width of a string,
// treating CJK characters as 2 columns and others as 1.
func VisualWidth(s string) int {
	width := 0
	for _, r := range s {
		if r >= 0x2e80 && r <= 0xa4cf || r >= 0xac00 && r <= 0xd7a3 ||
			r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe6f ||
			r >= 0xff00 && r <= 0xff60 || r >= 0xffe0 && r <= 0xffe6 ||
			r >= 0x20000 && r <= 0x3fffd {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// AlignLabel formats a label to a fixed visual column width (assuming CJK characters take 2 columns).
func AlignLabel(label string, targetWidth int) string {
	width := VisualWidth(label)
	if width >= targetWidth {
		return label
	}
	return label + strings.Repeat(" ", targetWidth-width)
}
