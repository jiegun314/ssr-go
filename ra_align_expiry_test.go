package main

// 对齐共享盘快照时也删失效注册证：旧版本发布的快照里可能还有过期行，
// 不能把它们带进整合。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/publish"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

func TestAlignDropsExpiredCertificates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := share.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	// 直接往共享盘发布一份"带过期行"的快照（模拟旧版本发布的版本）
	expired := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	_, err := publish.Publish(publish.Options{
		Root: root, Source: "ra_input",
		Columns: []string{"material_code", "ra_expiration_date"},
		Rows: []store.Row{
			{"material_code": "EXPIRED", "ra_expiration_date": expired},
			{"material_code": "VALID", "ra_expiration_date": future},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	app := newShareApp(t, root)
	result := app.AlignSource("ra_input")
	if result.Failed {
		t.Fatalf("对齐失败：%+v", result)
	}
	rows, err := app.repo.Rows("ra_input_staging")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["material_code"] != "VALID" {
		t.Fatalf("对齐后只应保留未过期的一行：%+v", rows)
	}
}
