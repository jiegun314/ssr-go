package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// 对齐动作的三种结果（与 Python 版 scripts/align_log_columns.py 的常量同名）。
const (
	AlignCreated        = "created"
	AlignAlreadyAligned = "already aligned"
	AlignAligned        = "aligned"
)

// stagingSuffix 是重写期间临时表的后缀：它只存在于那段事务里，回滚时一起消失。
const stagingSuffix = "__aligned"

// AlignReport 描述一次对齐做了什么。
type AlignReport struct {
	Action   string
	Database string
	Table    string
	Backup   string
	Rows     int
	Added    []string
	Removed  []string
}

// AlignDatabase 把一张日志表重写成配置的列与顺序，并保留所有已有行（R23）。
//
// 老版本建的库保留着那一版的列与列序：应用读得动（回顾窗口按配置列显示、缺的列留空），
// 但配置新增的列存不下东西。对齐就是按配置重建这张表。
//
// 重写前先备份（keepBackup 为 false 时跳过），重写在事务里完成并在提交前逐项校验，
// 因此失败时数据库保持原样。已经对齐的表不动。
func AlignDatabase(
	databasePath string,
	tableName string,
	columns []string,
	keepBackup bool,
) (AlignReport, error) {
	repository, err := Open(databasePath)
	if err != nil {
		return AlignReport{}, err
	}
	defer repository.Close()
	report := AlignReport{Database: databasePath, Table: tableName, Added: []string{}, Removed: []string{}}

	existing, err := repository.TableColumns(tableName)
	if err != nil {
		return report, err
	}
	switch {
	case len(existing) == 0:
		if err := repository.WriteRows(tableName, columns, nil, WriteReplace); err != nil {
			return report, err
		}
		report.Action = AlignCreated
		report.Added = append([]string{}, columns...)
		return report, nil
	case sameColumns(existing, columns):
		count, err := repository.CountTableRows(tableName)
		if err != nil {
			return report, err
		}
		report.Action = AlignAlreadyAligned
		report.Rows = count
		return report, nil
	}

	if keepBackup {
		backup, err := copyDatabase(repository, databasePath)
		if err != nil {
			return report, err
		}
		report.Backup = backup
	}
	if err := rewriteTable(repository, tableName, columns, &report); err != nil {
		return report, err
	}
	report.Action = AlignAligned
	return report, nil
}

// copyDatabase 用 SQLite 自己的备份机制复制一份一致的副本。
func copyDatabase(repository *Repository, databasePath string) (string, error) {
	stem := strings.TrimSuffix(filepath.Base(databasePath), filepath.Ext(databasePath))
	backupPath := filepath.Join(
		filepath.Dir(databasePath),
		fmt.Sprintf("%s-backup-%s%s", stem, time.Now().Format("20060102T150405"), filepath.Ext(databasePath)),
	)
	if err := repository.Execute("VACUUM INTO ?", backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

// rewriteTable 在事务里重建表并校验，任何异常都让事务整体回滚。
func rewriteTable(
	repository *Repository,
	tableName string,
	columns []string,
	report *AlignReport,
) error {
	existing, err := repository.TableColumns(tableName)
	if err != nil {
		return err
	}
	available := map[string]bool{}
	for _, name := range existing {
		available[name] = true
	}
	shared := []string{}
	for _, name := range existing {
		if containsString(columns, name) {
			shared = append(shared, name)
		}
	}
	added := []string{}
	for _, name := range columns {
		if !available[name] {
			added = append(added, name)
		}
	}
	rowCount, err := repository.CountTableRows(tableName)
	if err != nil {
		return err
	}
	// 索引与触发器会在旧表被删时一起消失，所以要先取出来；拿不到定义的（主键/唯一
	// 约束生成的自动索引）无法按配置列重建，直接拒绝而不是悄悄丢掉。
	objects, err := ownedObjectSQL(repository, tableName)
	if err != nil {
		return err
	}

	staging := tableName + stagingSuffix
	targetColumns := quoteColumns(columns)
	valueList := make([]string, 0, len(columns))
	for _, name := range columns {
		if available[name] {
			valueList = append(valueList, quoteIdentifier(name))
			continue
		}
		valueList = append(valueList, "'' AS "+quoteIdentifier(name))
	}

	if err := repository.InTransaction(func(transaction *sqlTransaction) error {
		if err := transaction.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdentifier(staging))); err != nil {
			return err
		}
		if err := transaction.Exec(createTableStatement(staging, columns)); err != nil {
			return err
		}
		if err := transaction.Exec(fmt.Sprintf(
			"INSERT INTO %s (%s) SELECT %s FROM %s",
			quoteIdentifier(staging), targetColumns,
			strings.Join(valueList, ", "), quoteIdentifier(tableName))); err != nil {
			return err
		}
		rewritten, err := transaction.Count(staging)
		if err != nil {
			return err
		}
		if rewritten != rowCount {
			return fmt.Errorf(
				"%s holds %d rows but the rewrite wrote %d of them; the database was left unchanged",
				tableName, rowCount, rewritten)
		}
		if len(added) > 0 {
			parts := make([]string, 0, len(added))
			for _, name := range added {
				parts = append(parts, fmt.Sprintf(
					"%s IS NOT NULL AND %s <> ''", quoteIdentifier(name), quoteIdentifier(name)))
			}
			taken, err := transaction.CountWhere(staging, strings.Join(parts, " OR "))
			if err != nil {
				return err
			}
			if taken > 0 {
				return fmt.Errorf(
					"%s took values for columns it does not store: %s; the database was left unchanged",
					tableName, strings.Join(added, ", "))
			}
		}
		for _, pair := range [][2]string{
			{tableName, staging}, {staging, tableName},
		} {
			differing, err := transaction.DifferingRowVersions(pair[0], pair[1], shared)
			if err != nil {
				return err
			}
			if differing > 0 {
				return fmt.Errorf(
					"%d row versions of %s differ after the rewrite (%s against %s); the database was left unchanged",
					differing, tableName, pair[1], pair[0])
			}
		}
		if err := transaction.Exec(fmt.Sprintf("DROP TABLE %s", quoteIdentifier(tableName))); err != nil {
			return err
		}
		if err := transaction.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s",
			quoteIdentifier(staging), quoteIdentifier(tableName))); err != nil {
			return err
		}
		for _, statement := range objects {
			if err := transaction.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	report.Rows = rowCount
	report.Added = added
	for _, name := range existing {
		if !containsString(columns, name) {
			report.Removed = append(report.Removed, name)
		}
	}
	return nil
}

// ownedObjectSQL 返回表的索引与触发器定义，顺带拒绝无法重建的自动索引。
func ownedObjectSQL(repository *Repository, tableName string) ([]string, error) {
	rows, err := repository.Query(
		`SELECT type, name, sql FROM sqlite_master
		 WHERE tbl_name = ? AND type IN ('index', 'trigger') ORDER BY type, name`,
		tableName)
	if err != nil {
		return nil, err
	}
	statements := []string{}
	for _, row := range rows {
		objectType := row[0]
		name := row[1]
		definition := row[2]
		if definition == "" {
			return nil, fmt.Errorf(
				"%s declares a constraint through %s %s; a rewrite would drop it. "+
					"Rebuild that table by hand instead.",
				tableName, objectType, name)
		}
		statements = append(statements, definition)
	}
	return statements, nil
}

func createTableStatement(tableName string, columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, name := range columns {
		parts = append(parts, quoteIdentifier(name)+" "+ColumnType)
	}
	return fmt.Sprintf("CREATE TABLE %s (\n    %s\n)",
		quoteIdentifier(tableName), strings.Join(parts, ",\n    "))
}

func quoteColumns(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, name := range columns {
		quoted = append(quoted, quoteIdentifier(name))
	}
	return strings.Join(quoted, ", ")
}

func sameColumns(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
