// Package store 是唯一的 SQL 出口（对应 Python 的 database/repositories）。
//
// 两条硬约束（AGENTS.md §4.2 / R8）：
//   - **所有列都是 TEXT**，读出来一律当字符串：`"1"` 与 `"1.0"` 的差异会直接改变
//     导出内容，所以不能交给类型系统去猜；
//   - 建表列顺序 = 配置顺序，`optional` 列即使来源表头里没有也照样建列。
package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go 驱动，CGO_ENABLED=0 也能构建
)

// Row 是一行数据：列名 → 文本值。SQL 的 NULL 读作空字符串。
type Row = map[string]string

// WriteMode 是一次写入是替换整表还是追加。
type WriteMode string

const (
	// WriteReplace 先删表再按给定列重建（replace_on_import: true）。
	WriteReplace WriteMode = "replace"
	// WriteAppend 表不存在则建、缺列则补列、追加时不去重（replace_on_import: false）。
	WriteAppend WriteMode = "append"
	// ColumnType 是本项目唯一的列类型。
	ColumnType = "TEXT"
)

// Repository 是通用 SQLite 访问层。
type Repository struct {
	Path string
	db   *sql.DB
}

// Open 打开（必要时创建）一个 SQLite 文件。
func Open(path string) (*Repository, error) {
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open database %s: %w", path, err)
	}
	// 单机单用户工具：一条连接就够，避免并发写时的锁竞争。
	handle.SetMaxOpenConns(1)
	if _, err := handle.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		handle.Close()
		return nil, fmt.Errorf("Failed to open database %s: %w", path, err)
	}
	return &Repository{Path: path, db: handle}, nil
}

// Close 关闭数据库连接。
func (repository *Repository) Close() error {
	if repository == nil || repository.db == nil {
		return nil
	}
	return repository.db.Close()
}

// Execute 执行一条语句（服务层偶尔需要直接跑 SQL）。
func (repository *Repository) Execute(query string, parameters ...any) error {
	if _, err := repository.db.Exec(query, parameters...); err != nil {
		return fmt.Errorf("Database query failed: %w", err)
	}
	return nil
}

// TableNames 返回表名，按名称排序。
func (repository *Repository) TableNames() ([]string, error) {
	rows, err := repository.db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("Failed to list tables: %w", err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// TableExists 报告一张表是否存在。
func (repository *Repository) TableExists(tableName string) (bool, error) {
	row := repository.db.QueryRow(
		`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName)
	var one int
	switch err := row.Scan(&one); err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, fmt.Errorf("Failed to inspect table %s: %w", tableName, err)
	}
}

// TableColumns 返回列名，顺序与建表时一致。
func (repository *Repository) TableColumns(tableName string) ([]string, error) {
	rows, err := repository.db.Query(
		fmt.Sprintf(`PRAGMA table_info(%s)`, quoteIdentifier(tableName)))
	if err != nil {
		return nil, fmt.Errorf("Failed to get columns for table %s: %w", tableName, err)
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var (
			index        int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// CountTableRows 统计行数；表还不存在时按 0 计（全新数据库读作空，不是错误）。
func (repository *Repository) CountTableRows(tableName string) (int, error) {
	exists, err := repository.TableExists(tableName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	row := repository.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteIdentifier(tableName)))
	count := 0
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("Failed to count rows of table %s: %w", tableName, err)
	}
	return count, nil
}

// Rows 按列顺序读出全部行（不去重、不折叠业务键）。
func (repository *Repository) Rows(tableName string) ([]Row, error) {
	columns, err := repository.TableColumns(tableName)
	if err != nil {
		return nil, err
	}
	rows, err := repository.db.Query(
		fmt.Sprintf(`SELECT * FROM %s`, quoteIdentifier(tableName)))
	if err != nil {
		return nil, fmt.Errorf("Failed to read table %s: %w", tableName, err)
	}
	defer rows.Close()
	results := []Row{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := Row{}
		for index, column := range columns {
			row[column] = cellText(values[index])
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// KeyedRows 按某个字段把行收成「键 → 行」，重复键时后出现的行胜出。
//
// 这与 Python 版 `get_table_data(table, index_field)` 一致：一对一来源按
// material_code 收口，同一产品代码出现多次时最后一条参与整合。
func (repository *Repository) KeyedRows(tableName string, indexField string) (map[string]Row, error) {
	rows, err := repository.Rows(tableName)
	if err != nil {
		return nil, err
	}
	keyed := map[string]Row{}
	for position, row := range rows {
		key := fmt.Sprint(position)
		if indexField != "" {
			key = row[indexField]
		}
		keyed[key] = row
	}
	return keyed, nil
}

// WriteRows 按 WriteMode 写入整表。
func (repository *Repository) WriteRows(
	tableName string,
	columns []string,
	rows []Row,
	mode WriteMode,
) error {
	if mode != WriteReplace && mode != WriteAppend {
		return fmt.Errorf("Failed to write table %s: Unsupported write mode: %s", tableName, mode)
	}
	if mode == WriteReplace {
		if err := repository.DropTable(tableName); err != nil {
			return err
		}
	}
	exists, err := repository.TableExists(tableName)
	if err != nil {
		return err
	}
	if !exists {
		if err := repository.createTable(tableName, columns); err != nil {
			return err
		}
	} else if err := repository.AddMissingColumns(tableName, columns); err != nil {
		return err
	}
	if len(rows) == 0 {
		// 空数据也要建表：导入 0 行时表存在、行数为 0
		return nil
	}
	return repository.insertRows(tableName, columns, rows)
}

// AddMissingColumns 给已存在的表补上缺的列（追加导入时用，R8）。
func (repository *Repository) AddMissingColumns(tableName string, columns []string) error {
	existing, err := repository.TableColumns(tableName)
	if err != nil {
		return err
	}
	missing := []string{}
	for _, column := range columns {
		if !containsString(existing, column) {
			missing = append(missing, column)
		}
	}
	for _, column := range missing {
		statement := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`,
			quoteIdentifier(tableName), quoteIdentifier(column), ColumnType)
		if _, err := repository.db.Exec(statement); err != nil {
			return fmt.Errorf("Failed to add column %s to table %s: %w", column, tableName, err)
		}
	}
	return nil
}

// DropTable 删表；表不存在时是空操作（R25 的幂等要求）。
func (repository *Repository) DropTable(tableName string) error {
	statement := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, quoteIdentifier(tableName))
	if _, err := repository.db.Exec(statement); err != nil {
		return fmt.Errorf("Failed to drop table %s: %w", tableName, err)
	}
	return nil
}

func (repository *Repository) createTable(tableName string, columns []string) error {
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, fmt.Sprintf("%s %s", quoteIdentifier(column), ColumnType))
	}
	statement := fmt.Sprintf("CREATE TABLE %s (\n    %s\n)",
		quoteIdentifier(tableName), strings.Join(definitions, ",\n    "))
	if _, err := repository.db.Exec(statement); err != nil {
		return fmt.Errorf("Failed to create table %s: %w", tableName, err)
	}
	return nil
}

func (repository *Repository) insertRows(tableName string, columns []string, rows []Row) error {
	quoted := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(column))
		placeholders = append(placeholders, "?")
	}
	statement := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdentifier(tableName),
		strings.Join(quoted, ", "),
		strings.Join(placeholders, ", "),
	)
	transaction, err := repository.db.Begin()
	if err != nil {
		return fmt.Errorf("Failed to write table %s: %w", tableName, err)
	}
	defer transaction.Rollback() //nolint:errcheck // 提交成功后回滚是空操作
	prepared, err := transaction.Prepare(statement)
	if err != nil {
		return fmt.Errorf("Failed to write table %s: %w", tableName, err)
	}
	defer prepared.Close()
	for _, row := range rows {
		values := make([]any, 0, len(columns))
		for _, column := range columns {
			values = append(values, row[column])
		}
		if _, err := prepared.Exec(values...); err != nil {
			return fmt.Errorf("Failed to write table %s: %w", tableName, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("Failed to write table %s: %w", tableName, err)
	}
	return nil
}

// cellText 把一列单元格读成文本：NULL 读作空字符串（R1 的「没写」）。
func cellText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

// quoteIdentifier 按 SQLite 的规则引用标识符（双引号内的双引号写两次）。
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
