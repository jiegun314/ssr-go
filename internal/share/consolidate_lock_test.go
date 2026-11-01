package share

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidateLockBlocksTheSecondUser(t *testing.T) {
	root := t.TempDir()
	owner := Owner{Name: "zhangsan", Role: "supply_chain"}
	if err := AcquireConsolidation(root, "id-1", owner); err != nil {
		t.Fatalf("第一次抢锁失败：%v", err)
	}
	state, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsolidationRunning == nil || state.ConsolidationRunning.ID != "id-1" {
		t.Fatalf("state 里应当有 running 记录：%+v", state.ConsolidationRunning)
	}
	// 第二个人应当被拦下，并看到"谁在整合"
	err = AcquireConsolidation(root, "id-2", Owner{Name: "lisi", Role: "supply_chain"})
	if err == nil || !strings.Contains(err.Error(), "zhangsan") {
		t.Fatalf("第二个人应当被拒并看到对方：%v", err)
	}
	// 释放后可以再抢
	ReleaseConsolidation(root, "id-1")
	if _, err := os.Stat(filepath.Join(root, "state", "consolidate.lock")); !os.IsNotExist(err) {
		t.Error("释放后锁文件应当被删掉")
	}
	state, _ = LoadState(root)
	if state.ConsolidationRunning != nil {
		t.Errorf("释放后 running 应当清空：%+v", state.ConsolidationRunning)
	}
	if err := AcquireConsolidation(root, "id-3", owner); err != nil {
		t.Fatalf("释放后应当可以再抢：%v", err)
	}
	ReleaseConsolidation(root, "id-3")
}
