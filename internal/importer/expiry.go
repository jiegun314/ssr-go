package importer

// 导入第一步：删除已失效的注册证（来源级配置 drop_expired.field）。
//
// 口径：
//   - 该列解析出的日期 **早于今天** 的行整行丢弃（今天到期仍算有效）；
//   - 空值 / 无法解析的值一律保留（不因为格式问题误删数据）；
//   - 原文表、清洗表、之后的发布快照都不再包含这些行。

import (
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/store"
)

// DropExpired 是给"对齐共享盘快照"复用的入口：删掉失效日期早于今天的行。
// 快照可能由旧版本发布（当时还没有这个过滤），对齐时同样要清掉，保证整合输入一致。
func DropExpired(field string, rows []store.Row, today time.Time) ([]store.Row, int) {
	kept, _, dropped := dropExpiredRows(field, rows, nil, today)
	return kept, dropped
}

// dropExpiredRows 返回过滤后的记录、原始单元格与删除行数（两者一一对应）。
func dropExpiredRows(
	field string,
	records []store.Row,
	rawCells []map[string]string,
	today time.Time,
) ([]store.Row, []map[string]string, int) {
	cutoff := startOfDay(today)
	keptRecords := make([]store.Row, 0, len(records))
	keptRaw := make([]map[string]string, 0, len(rawCells))
	dropped := 0
	for index, record := range records {
		value := strings.TrimSpace(record[field])
		if value != "" {
			if expiry, ok := parseDateValue(value); ok && expiry.Before(cutoff) {
				dropped++
				continue
			}
		}
		keptRecords = append(keptRecords, record)
		if index < len(rawCells) {
			keptRaw = append(keptRaw, rawCells[index])
		}
	}
	return keptRecords, keptRaw, dropped
}

// parseDateValue 解析 Excel 里常见的日期写法（只取日期部分）。
func parseDateValue(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	if index := strings.IndexAny(text, " T"); index > 0 {
		text = strings.TrimSpace(text[:index])
	}
	layouts := []string{
		"2006-01-02",
		"2006/01/02",
		"2006.01.02",
		"2006-1-2",
		"2006/1/2",
		"20060102",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func startOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

// expiryFieldOf 读 drop_expired 块（只认 field；缺块/缺字段 = 不做过滤）。
func expiryFieldOf(raw any) string {
	block, ok := raw.(map[string]any)
	if !ok || block == nil {
		return ""
	}
	return textOf(block["field"])
}
