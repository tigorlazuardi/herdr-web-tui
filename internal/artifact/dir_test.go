package artifact

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDefaultDir_UsesNumericProcessUID(t *testing.T) {
	prefix := "herdr-web-tui-test-" + strconv.Itoa(os.Getpid())
	dir, err := DefaultDir(prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	want := filepath.Join(os.TempDir(), prefix+"-"+strconv.Itoa(os.Geteuid()))
	if dir != want {
		t.Fatalf("DefaultDir() = %q, want %q", dir, want)
	}
}
