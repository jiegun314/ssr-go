package oplogsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishPullRoundTrip(t *testing.T) {
	root := t.TempDir()
	columns := []string{"material_code", "log_time"}
	rows := [][]string{{"M1", "2026-11-01 10:00:00"}, {"M2", "2026-11-01 10:00:00"}}
	manifest, err := Publish(root, columns, rows, "zhangsan")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Rows != 2 || manifest.SHA256 == "" {
		t.Fatalf("manifest 不对：%+v", manifest)
	}
	snapshot, err := Pull(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Rows) != 2 || snapshot.Rows[0][0] != "M1" {
		t.Fatalf("拉回来的内容不对：%+v", snapshot)
	}
	// 没有 manifest 时返回 os.ErrNotExist（调用方据此初始化）
	if _, err := Pull(t.TempDir()); !os.IsNotExist(err) {
		t.Errorf("没有日志时应当是 os.ErrNotExist：%v", err)
	}
}

func TestMergeDedupesByRowContent(t *testing.T) {
	left := [][]string{{"M1", "t1"}, {"M2", "t1"}}
	right := [][]string{{"M2", "t1"}, {"M3", "t2"}}
	merged, added := Merge(left, right)
	if len(merged) != 3 || added != 1 {
		t.Fatalf("合并结果不对：len=%d added=%d %+v", len(merged), added, merged)
	}
	// 再合并一次不会增加
	again, addedAgain := Merge(merged, right)
	if len(again) != 3 || addedAgain != 0 {
		t.Fatalf("重复合并应当不增加：%d/%d", len(again), addedAgain)
	}
	_ = filepath.Join
}
