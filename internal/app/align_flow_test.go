package app

// P3：对齐（拉取 + 校验 + 覆盖导入）与"待整合"状态。
//
//   - 第二个实例从共享盘对齐 → 本机表被整表覆盖 → 总览显示已对齐；
//   - 整合前：四份必须都对齐，否则拒绝；整合成功后写 last_consolidation，"待整合"清除；
//   - 再有新版本发布 → 重新变成"未对齐/待整合"；
//   - 共享盘快照被篡改 → 对齐失败（sha256 不符）。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// publishSamples 让一个实例把四份样本都导入并发布到共享盘。
func publishSamples(t *testing.T, app *App) {
	t.Helper()
	for _, source := range []string{"ra_input", "global_udi_input", "medical_insurance_code", "product_category"} {
		result := app.ImportSource(source, filepath.Join("testdata", "sample-valid", source+".xlsx"))
		if result.Failed {
			t.Fatalf("导入并发布 %s 失败：%+v", source, result)
		}
	}
}

func TestAlignThenConsolidateAndPendingClears(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	publisher := newShareApp(t, shareRoot)
	publishSamples(t, publisher)

	consumer := newShareApp(t, shareRoot)
	overview := consumer.ShareOverview()
	if !overview.Enabled || len(overview.Sources) != 4 {
		t.Fatalf("总览应当有四个来源：%+v", overview)
	}
	for _, source := range overview.Sources {
		if source.Aligned {
			t.Errorf("%s 还没对齐，不该显示已对齐", source.Label)
		}
		if source.RemoteVersion == "" {
			t.Errorf("%s 应当有共享盘版本（单机模式是 Admin，能看全部）", source.Label)
		}
	}

	// 未对齐前不许整合
	if result := consumer.Consolidate(); !result.Failed || !strings.Contains(result.Message, "未对齐") {
		t.Fatalf("未对齐时整合应当被拒：%+v", result)
	}

	// 全部对齐
	aligned := consumer.AlignAll()
	if aligned.Failed {
		t.Fatalf("对齐应当成功：%+v", aligned)
	}
	if len(aligned.Aligned) != 4 {
		t.Fatalf("应当对齐四份，实际 %v", aligned.Aligned)
	}
	synced, err := consumer.repo.SyncedVersions()
	if err != nil || len(synced) != 4 {
		t.Fatalf("synced_versions 应当有四个来源：%v / %v", synced, err)
	}
	// RA 的清洗表也要按规则重建
	if count, err := consumer.repo.CountTableRows("ra_input_cleaned"); err != nil || count != 6 {
		t.Errorf("RA 清洗表应当重建为 6 行：%d / %v", count, err)
	}

	overview = consumer.ShareOverview()
	for _, source := range overview.Sources {
		if !source.Aligned {
			t.Errorf("%s 对齐后应当显示已对齐", source.Label)
		}
		if !source.Pending {
			t.Errorf("%s 还没整合，应当显示待整合", source.Label)
		}
	}

	// 对齐后可以整合，并写入 last_consolidation
	if result := consumer.Consolidate(); result.Failed {
		t.Fatalf("对齐后整合应当成功：%+v", result)
	}
	overview = consumer.ShareOverview()
	if overview.LastConsolidation == nil || overview.LastConsolidation.At == "" {
		t.Fatal("整合后应当有最后整合时间")
	}
	for _, source := range overview.Sources {
		if source.Pending {
			t.Errorf("%s 整合后不该再显示待整合", source.Label)
		}
	}

	// 发布者再发一版 RA → 消费端重新变成未对齐 + 待整合
	// （版本号精确到秒：等一下，确保这次发布得到新版本号）
	time.Sleep(1100 * time.Millisecond)
	result := publisher.ImportSource("ra_input", filepath.Join("testdata", "sample-valid", "ra_input.xlsx"))
	if result.Failed {
		t.Fatalf("再次发布失败：%+v", result)
	}
	overview = consumer.ShareOverview()
	for _, source := range overview.Sources {
		if source.Source != "ra_input" {
			continue
		}
		if source.Aligned {
			t.Error("新版本发布后应当显示未对齐")
		}
		if !source.Pending {
			t.Error("新版本发布后应当显示待整合")
		}
	}
	if result := consumer.Consolidate(); !result.Failed || !strings.Contains(result.Message, "未对齐") {
		t.Fatalf("有新版本未对齐时整合应当被拒：%+v", result)
	}
}

func TestAlignRejectsTamperedSnapshot(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	publisher := newShareApp(t, shareRoot)
	publishSamples(t, publisher)
	consumer := newShareApp(t, shareRoot)

	// 篡改 RA 的快照内容（sha256 会不符）
	state, err := consumer.shareState()
	if err != nil {
		t.Fatalf("读状态失败：%v", err)
	}
	version := state.Sources["ra_input"].Version
	csvPath := filepath.Join(shareRoot, "published", "ra", version+".csv")
	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, append(content, []byte("000MAT-999,被篡改,注入行\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	result := consumer.AlignSource("ra_input")
	if !result.Failed || !strings.Contains(result.Message, "校验失败") {
		t.Fatalf("篡改的快照应当被拒：%+v", result)
	}
}

func TestOverviewHidesOtherSourcesFromRa(t *testing.T) {
	shareRoot := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	publisher := newShareApp(t, shareRoot)
	publishSamples(t, publisher)

	// 带 users.yaml 的实例：登录 RA
	workspace := t.TempDir()
	copyDirectoryForTest(t, "config", filepath.Join(workspace, "config"))
	if err := os.WriteFile(filepath.Join(workspace, "config", "users.yaml"), []byte(testUsersYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	workflow := "share:\n  root: '" + shareRoot + "'\n"
	if err := os.WriteFile(filepath.Join(workspace, "config", "workflow.yaml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDI_CONFIG_DIR", filepath.Join(workspace, "config"))
	app := NewApp(nil)
	app.startup(context.Background())
	if login := app.Login("ra", ""); login.Failed {
		t.Fatalf("RA 登录失败：%+v", login)
	}

	overview := app.ShareOverview()
	own, other := 0, 0
	for _, source := range overview.Sources {
		switch source.Source {
		case "ra_input":
			own++
			if !source.CanView || source.RemoteVersion == "" || source.RemoteRows == 0 || source.PublishedBy == "" {
				t.Errorf("RA 应当看到自己来源的明细：%+v", source)
			}
			if source.PublishedAt == "" {
				t.Error("RA 自己的来源也要有最后更新时间")
			}
		default:
			other++
			if source.CanView {
				t.Errorf("%s：RA 不该有查看权限", source.Label)
			}
			if source.RemoteVersion != "" || source.RemoteRows != 0 || source.PublishedBy != "" {
				t.Errorf("%s：无权查看时不该下发明细：%+v", source.Label, source)
			}
			if source.PublishedAt == "" {
				t.Errorf("%s：应当下发最后更新时间", source.Label)
			}
		}
	}
	if own != 1 || other != 3 {
		t.Fatalf("来源数不对：own=%d other=%d", own, other)
	}
}
