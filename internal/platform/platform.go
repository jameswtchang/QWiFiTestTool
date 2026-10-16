// Package platform holds dependency-free runtime platform checks shared by all packages
// (models, sys and config cannot share a helper otherwise: sys imports models).
package platform

import (
	"os"
	"runtime"
)

// androidBuildProp exists on every Android system image (overridable in tests)
var androidBuildProp = "/system/build.prop"

// IsAndroid reports whether the tool runs on Android: either built for GOOS=android,
// or (the usual case, built as GOOS=linux) running on a system with /system/build.prop.
func IsAndroid() bool {
	if runtime.GOOS == "android" {
		return true
	}
	_, err := os.Stat(androidBuildProp)
	return err == nil
}
