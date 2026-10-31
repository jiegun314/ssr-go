package store

import (
	"fmt"
	"strconv"
)

// SyncTable 是"本机已对齐到哪一版"的本地记录表。
//
// 与共享盘的 state.json 不同：这里只记本机事实（我这张表当前是哪一版），
// 用于界面显示"本机版本 vs 共享盘版本"，以及整合前的对齐校验。
const SyncTable = "synced_versions"

var syncColumns = []string{"source", "version", "sha256", "rows", "synced_at", "synced_by"}

// SyncRecord 是一个来源的本机对齐记录。
type SyncRecord struct {
	Source   string
	Version  string
	SHA256   string
	Rows     int
	SyncedAt string
	SyncedBy string
}

// SyncedVersions 读本机记录（表不存在/为空时返回空 map）。
func (repository *Repository) SyncedVersions() (map[string]SyncRecord, error) {
	exists, err := repository.TableExists(SyncTable)
	if err != nil {
		return nil, err
	}
	records := map[string]SyncRecord{}
	if !exists {
		return records, nil
	}
	rows, err := repository.Rows(SyncTable)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		source := row["source"]
		if source == "" {
			continue
		}
		count, _ := strconv.Atoi(row["rows"])
		records[source] = SyncRecord{
			Source: source, Version: row["version"], SHA256: row["sha256"], Rows: count,
			SyncedAt: row["synced_at"], SyncedBy: row["synced_by"],
		}
	}
	return records, nil
}

// SaveSyncedVersion 记下一次对齐（同一来源覆盖旧记录，其余保留）。
func (repository *Repository) SaveSyncedVersion(record SyncRecord) error {
	if record.Source == "" || record.Version == "" {
		return fmt.Errorf("来源与版本不能为空")
	}
	records, err := repository.SyncedVersions()
	if err != nil {
		return err
	}
	records[record.Source] = record
	rows := make([]Row, 0, len(records))
	for _, item := range records {
		rows = append(rows, Row{
			"source": item.Source, "version": item.Version, "sha256": item.SHA256,
			"rows": strconv.Itoa(item.Rows), "synced_at": item.SyncedAt, "synced_by": item.SyncedBy,
		})
	}
	return repository.WriteRows(SyncTable, syncColumns, rows, WriteReplace)
}
