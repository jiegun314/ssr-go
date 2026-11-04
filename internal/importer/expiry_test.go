package importer

import (
	"testing"
	"time"

	"github.com/jiegun314/ssr-go/internal/store"
)

func TestParseDateValueAcceptsCommonFormats(t *testing.T) {
	for _, value := range []string{
		"2026-11-04", "2026/11/04", "2026.11.04", "2026-1-2", "2026/1/2",
		"20261104", "2026-11-04 00:00:00", " 2026-11-04 ",
	} {
		if _, ok := parseDateValue(value); !ok {
			t.Errorf("应当能解析日期：%q", value)
		}
	}
	for _, value := range []string{"", "   ", "not-a-date", "2026年11月4日"} {
		if _, ok := parseDateValue(value); ok {
			t.Errorf("不该把 %q 当成日期", value)
		}
	}
}

func TestDropExpiredRowsKeepsTodayAndLater(t *testing.T) {
	today := time.Date(2026, 11, 5, 10, 30, 0, 0, time.Local)
	records := []store.Row{
		{"material_code": "EXPIRED", "ra_expiration_date": "2026-11-04"},
		{"material_code": "TODAY", "ra_expiration_date": "2026/11/05"},
		{"material_code": "FUTURE", "ra_expiration_date": "2027-01-01"},
		{"material_code": "EMPTY", "ra_expiration_date": ""},
		{"material_code": "BAD", "ra_expiration_date": "2026年11月1日"},
	}
	raw := make([]map[string]string, len(records))
	for index, record := range records {
		raw[index] = map[string]string{"ra_expiration_date": record["ra_expiration_date"]}
	}
	kept, keptRaw, dropped := dropExpiredRows("ra_expiration_date", records, raw, today)
	if dropped != 1 {
		t.Fatalf("只应删除 1 行（昨日到期），实际 %d", dropped)
	}
	codes := []string{}
	for _, record := range kept {
		codes = append(codes, record["material_code"])
	}
	want := []string{"TODAY", "FUTURE", "EMPTY", "BAD"}
	if len(codes) != len(want) {
		t.Fatalf("保留行数不对：%v", codes)
	}
	for index := range want {
		if codes[index] != want[index] {
			t.Errorf("保留行顺序不对：%v", codes)
			break
		}
	}
	if len(keptRaw) != len(kept) {
		t.Errorf("原文单元格与记录应当一一对应：%d / %d", len(keptRaw), len(kept))
	}
}
