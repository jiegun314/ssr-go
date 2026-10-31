package share

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidRootAcceptsAbsoluteAndUnc(t *testing.T) {
	cases := map[string]bool{
		"":               true, // 本地模式
		"/tmp/ssr-share": true,
		`\\server\share`: true,
		"relative/share": false,
		"./share":        false,
	}
	for root, want := range cases {
		if got := ValidRoot(root); got != want {
			t.Errorf("ValidRoot(%q) = %v；want %v", root, got, want)
		}
	}
}

func TestLoadStateReturnsEmptyForFreshRoot(t *testing.T) {
	root := t.TempDir()
	state, err := LoadState(root)
	if err != nil {
		t.Fatalf("读空状态失败：%v", err)
	}
	if state.Generation != 0 || len(state.Sources) != 0 {
		t.Fatalf("空状态应当 generation=0 且没有来源：%+v", state)
	}
}

func TestStateRoundTripAndAtomicSave(t *testing.T) {
	root := t.TempDir()
	state := &State{Sources: map[string]*SourceVersion{
		"ra_input": {Version: "20260101T000000-abcdef12", SHA256: "abc", Rows: 3,
			PublishedAt: "2026-01-01 00:00:00", PublishedBy: Owner{Name: "zhangsan", Role: "ra"}},
	}}
	state.Generation = 7
	state.LastConsolidation = &Consolidation{ID: "c1", At: "2026-01-01 01:00:00",
		Versions: map[string]string{"ra_input": "20260101T000000-abcdef12"},
		Counts:   Counts{Ready: 1}, Email: EmailStatus{Status: "disabled"}}
	if err := state.Save(root); err != nil {
		t.Fatalf("保存失败：%v", err)
	}
	// 原子写：不留下 .tmp
	entries, _ := os.ReadDir(filepath.Join(root, "state"))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("不该留下临时文件：%s", entry.Name())
		}
	}
	loaded, err := LoadState(root)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if loaded.Generation != 7 || loaded.Sources["ra_input"].Version != "20260101T000000-abcdef12" {
		t.Fatalf("读回内容不对：%+v", loaded)
	}
	if loaded.LastConsolidation == nil || loaded.LastConsolidation.Email.Status != "disabled" {
		t.Errorf("last_consolidation 没读回来：%+v", loaded.LastConsolidation)
	}
}

func TestWithStateLockSerialisesAndCleansUp(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithStateLock(root, func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	// 锁被占用时，第二个调用应当等待而不是同时进入
	waited := time.Now()
	go func() {
		time.Sleep(120 * time.Millisecond)
		close(release)
	}()
	if err := WithStateLock(root, func() error { return nil }); err != nil {
		t.Fatalf("第二个调用应当等到锁释放：%v", err)
	}
	if elapsed := time.Since(waited); elapsed < 100*time.Millisecond {
		t.Errorf("第二个调用没有等待（%v），说明锁没生效", elapsed)
	}
	<-done
	if _, err := os.Stat(filepath.Join(root, "state", "state.lock")); !os.IsNotExist(err) {
		t.Error("锁文件应当被删掉")
	}
}

func TestWithStateLockRecoversStaleLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "state", "state.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := WithStateLock(root, func() error { called = true; return nil }); err != nil {
		t.Fatalf("过期的锁应当被回收：%v", err)
	}
	if !called {
		t.Error("回收后应当执行回调")
	}
}

func TestDepartmentMapping(t *testing.T) {
	cases := map[string]string{
		"ra_input": "ra", "product_category": "eama", "medical_insurance_code": "eama",
		"global_udi_input": "udi",
	}
	for source, want := range cases {
		if got := Department(source); got != want {
			t.Errorf("Department(%q) = %q；want %q", source, got, want)
		}
	}
}
