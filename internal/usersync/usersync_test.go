package usersync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishAndFetch(t *testing.T) {
	root := t.TempDir()
	content := []byte("users:\n  - name: \"张三\"\n    role: \"ra\"\n")
	manifest, err := Publish(root, content, "admin")
	if err != nil {
		t.Fatalf("发布失败：%v", err)
	}
	if manifest.SHA256 != Hash(content) || manifest.Version == "" {
		t.Errorf("manifest 内容不对：%+v", manifest)
	}
	got, fetched, err := Fetch(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) || fetched.SHA256 != manifest.SHA256 {
		t.Errorf("拉取内容不对：%+v", fetched)
	}
	// 内容相同不重复发布
	again, err := Publish(root, content, "admin")
	if err != nil || again.Version != manifest.Version {
		t.Errorf("相同内容不应产生新版本：%+v / %v", again, err)
	}
}

func TestFetchDetectsTampering(t *testing.T) {
	root := t.TempDir()
	if _, err := Publish(root, []byte("users:\n  - name: \"张三\"\n    role: \"ra\"\n"), "admin"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, Directory, FileName)
	if err := os.WriteFile(path, []byte("users:\n  - name: \"李四\"\n    role: \"ra\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Fetch(root); err == nil {
		t.Error("内容被改过时必须校验失败")
	}
}

func TestFetchMissing(t *testing.T) {
	if _, _, err := Fetch(t.TempDir()); err != ErrNotFound {
		t.Errorf("没有名单时应当是 ErrNotFound：%v", err)
	}
}
