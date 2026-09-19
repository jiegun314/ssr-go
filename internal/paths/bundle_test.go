package paths

import (
	"path/filepath"
	"testing"
)

func TestBundleParentFindsTheDirectoryHoldingTheApp(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "Users", "someone", "Applications")
	executable := filepath.Join(root, "SingleSourceReady.app", "Contents", "MacOS", "ssr")

	directory, ok := bundleParent(executable)

	if !ok {
		t.Fatal("应用包里的可执行文件应当被识别出来")
	}
	if directory != root {
		t.Fatalf("配置目录基准 = %q; want %q", directory, root)
	}
}

func TestBundleParentIgnoresACheckout(t *testing.T) {
	executable := filepath.Join(string(filepath.Separator), "repo", "build", "bin", "ssr")
	if _, ok := bundleParent(executable); ok {
		t.Fatal("普通目录里的可执行文件不该被当成应用包")
	}
}
