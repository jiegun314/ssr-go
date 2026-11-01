package main

// 操作日志的全量同步（用户反馈：记录导出所有人可见 → 日志全量同步）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationLogSyncsAcrossMachines(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	machineA := newShareApp(t, shareRoot)
	machineB := newShareApp(t, shareRoot)

	exportOn := func(app *App) {
		t.Helper()
		if result := app.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx")); result.Failed {
			t.Fatalf("导入失败：%+v", result)
		}
		for _, source := range []string{"global_udi_input", "medical_insurance_code", "product_category"} {
			if result := app.ImportSource(source, filepath.Join("testdata", "sample-valid", source+".xlsx")); result.Failed {
				t.Fatalf("导入 %s 失败：%+v", source, result)
			}
		}
		if result := app.AlignAll(); result.Failed {
			t.Fatalf("对齐失败：%+v", result)
		}
		if result := app.Consolidate(); result.Failed {
			t.Fatalf("整合失败：%+v", result)
		}
		if result := app.Export("", filepath.Join(t.TempDir(), "result.xlsx")); result.Failed {
			t.Fatalf("导出失败：%+v", result)
		}
	}

	exportOn(machineA)
	first := machineA.ReviewLog("", "", 1, 1000)
	if first.Failed || first.Total == 0 {
		t.Fatalf("A 导出后应当有记录：%+v", first)
	}
	// 共享盘上应当已经有日志
	if _, err := os.Stat(filepath.Join(shareRoot, "log", "operation_log.csv")); err != nil {
		t.Fatalf("导出后应当把日志同步到共享盘：%v", err)
	}

	// B 登录后第一次看「记录导出」：应当先同步到 A 的全量
	if login := machineB.Login("", ""); !login.Failed {
		// 单机模式登录直接返回当前身份（不失败），这里无需断言
	}
	second := machineB.ReviewLog("", "", 1, 1000)
	if second.Failed {
		t.Fatalf("B 读记录失败：%+v", second)
	}
	if second.Total != first.Total {
		t.Fatalf("B 应当看到与 A 相同的全量记录：A=%d B=%d", first.Total, second.Total)
	}

	// B 也导出一次 → 记录追加；A 再同步一次应当看到 B 的追加
	// （日志时间精确到秒：等一下，确保这次的记录与 A 的不同，能看出"追加"）
	time.Sleep(1100 * time.Millisecond)
	exportOn(machineB)
	afterB := machineB.ReviewLog("", "", 1, 1000)
	if afterB.Total <= first.Total {
		t.Fatalf("B 导出后记录应当变多：%d", afterB.Total)
	}
	afterA := machineA.ReviewLog("", "", 1, 1000)
	if afterA.Total != afterB.Total {
		t.Fatalf("A 应当看到与 B 相同的全量记录：A=%d B=%d", afterA.Total, afterB.Total)
	}
	// 重复同步不应当重复追加
	again := machineA.ReviewLog("", "", 1, 1000)
	if again.Total != afterA.Total {
		t.Fatalf("重复同步不应当增加记录：%d → %d", afterA.Total, again.Total)
	}
	_ = strings.TrimSpace
}
