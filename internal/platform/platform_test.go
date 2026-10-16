package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsAndroid(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("GOOS=android always reports true")
	}
	orig := androidBuildProp
	t.Cleanup(func() { androidBuildProp = orig })

	androidBuildProp = filepath.Join(t.TempDir(), "missing", "build.prop")
	if IsAndroid() {
		t.Error("expected false without build.prop")
	}

	prop := filepath.Join(t.TempDir(), "build.prop")
	if err := os.WriteFile(prop, []byte("ro.build.version.release=14\n"), 0644); err != nil {
		t.Fatal(err)
	}
	androidBuildProp = prop
	if !IsAndroid() {
		t.Error("expected true with build.prop present")
	}
}
