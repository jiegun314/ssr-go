package store

import (
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/config"
)

// logOptions 从真实配置里读出日志表的参数（与运行期完全同源）。
func logOptions(t *testing.T, loader *config.Loader) OperationLogOptions {
	t.Helper()
	setting, err := loader.LoadSetting()
	if err != nil {
		t.Fatalf("读 setting.yaml 失败：%v", err)
	}
	tables, _ := setting["tables"].(map[string]any)
	operationLog, _ := tables["operation_log"].(map[string]any)
	tableName, _ := operationLog["name"].(string)
	timeColumn, _ := tables["operation_log_time_column"].(string)
	identityFields, err := loader.LoadDuplicateCheckIdentityFields()
	if err != nil {
		t.Fatalf("读身份字段失败：%v", err)
	}
	logColumns, err := loader.LoadLogColumns()
	if err != nil {
		t.Fatalf("读日志列失败：%v", err)
	}
	return OperationLogOptions{
		TableName:      tableName,
		TimeColumn:     timeColumn,
		IdentityFields: identityFields,
		LogColumns:     logColumns,
	}
}

func newOperationLog(t *testing.T, repository *Repository, loader *config.Loader) *OperationLog {
	t.Helper()
	log, err := OpenOperationLog(repository, logOptions(t, loader))
	if err != nil {
		t.Fatalf("打开日志表失败：%v", err)
	}
	return log
}

func fixedClock(stamp string) func() string {
	return func() string { return stamp }
}

func TestR23CreatesTheMissingLogTable(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)

	log := newOperationLog(t, repository, loader)

	exists, err := repository.TableExists(log.TableName)
	if err != nil || !exists {
		t.Fatalf("缺表时应按配置建一张空表（err=%v）", err)
	}
	columns, err := repository.TableColumns(log.TableName)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(columns, "|") != strings.Join(log.TableColumns(), "|") {
		t.Fatalf("建表列顺序必须等于配置顺序")
	}
	if len(columns) != 80 {
		t.Fatalf("operation_log 共 80 列（2 + 75 + 3），得到 %d", len(columns))
	}
	identity, err := log.ReadIdentityFields()
	if err != nil {
		t.Fatalf("读身份字段失败：%v", err)
	}
	if len(identity) != 0 {
		t.Fatalf("新建的日志表应当是空的：%v", identity)
	}

	log.Now = fixedClock("2026/01/01 00:00:00")
	if err := log.InsertOperation("export", "created the missing log table"); err != nil {
		t.Fatalf("写操作日志失败：%v", err)
	}
	records, err := log.ReadByTime("2000/01/01 00:00:00", "2999/12/31 23:59:59")
	if err != nil {
		t.Fatalf("读日志失败：%v", err)
	}
	if len(records) != 1 {
		t.Fatalf("记录数 = %d; want 1", len(records))
	}
	if records[0]["operation"] != "export" || records[0]["details"] != "created the missing log table" {
		t.Fatalf("记录内容不对：%v", records[0])
	}
	// 回顾永远列出全部配置列，缺的列显示为空（不是 None / NaN）
	if len(records[0]) != len(log.TableColumns()) {
		t.Fatalf("回顾列数 = %d; want %d", len(records[0]), len(log.TableColumns()))
	}
	if records[0]["Package Status"] != "" {
		t.Fatalf("表里没有的列应当显示为空：%q", records[0]["Package Status"])
	}
}

func TestR23LeavesAnExistingLogTableAlone(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	identity := []string{"Catalog or Reference Number", "Primary DI"}
	legacyColumns := append([]string{}, identity...)
	legacyColumns = append(legacyColumns, "operation", "details", "log_time")
	if err := repository.WriteRows("operation_log", legacyColumns,
		[]Row{{
			"Catalog or Reference Number": "MAT-1",
			"Primary DI":                  "DI-1",
			"operation":                   "import",
			"details":                     "old row",
			"log_time":                    "2026/01/01 00:00:00",
		}}, WriteReplace); err != nil {
		t.Fatalf("建旧表失败：%v", err)
	}

	log := newOperationLog(t, repository, loader)

	columns, err := repository.TableColumns(log.TableName)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	// 已存在的表不被改动（R23）：不会补齐那 75 个导出列
	if strings.Join(columns, "|") != strings.Join(legacyColumns, "|") {
		t.Fatalf("旧表被改动了：%v", columns)
	}
	records, err := log.ReadByTime("2000/01/01 00:00:00", "2999/12/31 23:59:59")
	if err != nil {
		t.Fatalf("读日志失败：%v", err)
	}
	if len(records) != 1 || records[0]["operation"] != "import" {
		t.Fatalf("旧记录应当保留：%v", records)
	}
	// 旧表没有的列读作空
	if records[0]["Product Name/Generic Name"] != "" {
		t.Fatalf("旧表缺的列应为空：%q", records[0]["Product Name/Generic Name"])
	}
}

func TestR22AppendRecordsWritesEveryConfiguredColumn(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	log := newOperationLog(t, repository, loader)
	log.Now = fixedClock("2026/01/01 00:00:00")

	record := OperationLogRecord{
		Key: "MAT-001",
		Values: Row{
			"Catalog or Reference Number": "MAT-001",
			"Primary DI":                  "DI-001",
			"Product Name/Generic Name":   "Device name",
		},
	}
	if err := log.AppendRecords([]OperationLogRecord{record}, "material_code"); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	columns, err := repository.TableColumns(log.TableName)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	if strings.Join(columns, "|") != strings.Join(log.TableColumns(), "|") {
		t.Fatalf("写入后的列顺序应当是配置顺序")
	}
	rows, err := repository.Rows(log.TableName)
	if err != nil {
		t.Fatalf("读行失败：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("记录数 = %d", len(rows))
	}
	stored := rows[0]
	if stored["Product Name/Generic Name"] != "Device name" {
		t.Errorf("值没写进去：%q", stored["Product Name/Generic Name"])
	}
	if stored["material_code"] != "MAT-001" {
		t.Errorf("基准键应写进 material_code：%q", stored["material_code"])
	}
	// 整合行没提供的列写空（不是 NULL）
	if stored["Package Status"] != "" || stored["status"] != "" {
		t.Errorf("未提供的列应当为空：%q / %q", stored["Package Status"], stored["status"])
	}
}

func TestR17ReadLatestRecordsKeepsTheNewestPerIdentity(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	log := newOperationLog(t, repository, loader)
	columns := []string{
		"Catalog or Reference Number", "Primary DI", "Product Name/Generic Name", "log_time",
	}
	if err := repository.WriteRows("operation_log", columns, []Row{
		{"Catalog or Reference Number": "value-old", "Primary DI": "DI-1",
			"Product Name/Generic Name": "Old name", "log_time": "2026/01/01 00:00:00"},
		{"Catalog or Reference Number": "value-old", "Primary DI": "DI-1",
			"Product Name/Generic Name": "New name", "log_time": "2026/02/02 00:00:00"},
		{"Catalog or Reference Number": "value-other", "Primary DI": "DI-2",
			"Product Name/Generic Name": "Other name", "log_time": "2026/01/15 00:00:00"},
	}, WriteAppend); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	latest, err := log.ReadLatestRecords()
	if err != nil {
		t.Fatalf("读最新记录失败：%v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("同一身份只留一条，得到 %d 条", len(latest))
	}
	byIdentity := map[string]Row{}
	for _, row := range latest {
		byIdentity[row["Catalog or Reference Number"]] = row
	}
	if byIdentity["value-old"]["Product Name/Generic Name"] != "New name" {
		t.Errorf("应取时间最新的一条：%v", byIdentity["value-old"])
	}
	if byIdentity["value-other"]["Product Name/Generic Name"] != "Other name" {
		t.Errorf("另一个身份不受影响：%v", byIdentity["value-other"])
	}
}

func TestR17OnlyStoredColumnsCanProveAChange(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	if err := repository.WriteRows("operation_log",
		[]string{"Catalog or Reference Number", "Primary DI", "operation", "details", "log_time"},
		[]Row{{"operation": "import", "details": "log table without an exported record",
			"log_time": "2026/01/01 00:00:00"}}, WriteReplace); err != nil {
		t.Fatalf("建表失败：%v", err)
	}
	log := newOperationLog(t, repository, loader)

	latest, err := log.ReadLatestRecords()
	if err != nil {
		t.Fatalf("读最新记录失败：%v", err)
	}
	if len(latest) != 1 {
		t.Fatalf("记录数 = %d", len(latest))
	}
	if _, present := latest[0]["operation"]; !present {
		t.Error("表里真的有的列应当读出来")
	}
	if _, present := latest[0]["DI Record Publish Date"]; present {
		t.Error("存储里没有的列不能拿来比较（也就不能证明变化）")
	}
}

func TestR19MissingIdentityColumnsAreReported(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	options := logOptions(t, loader)
	options.IdentityFields = []string{"Not A Log Column"}
	log, err := OpenOperationLog(repository, options)
	if err != nil {
		t.Fatalf("打开日志表失败：%v", err)
	}

	_, err = log.ReadIdentityFields()
	if err == nil || !strings.Contains(err.Error(),
		"missing configured duplicate check columns") {
		t.Fatalf("缺少身份列时要明确报错，得到：%v", err)
	}
}

func TestR22UnchangedRecordsAreNotAppendedAgain(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	log := newOperationLog(t, repository, loader)
	log.Now = fixedClock("2026/01/01 00:00:00")
	record := OperationLogRecord{
		Key: "MAT-001",
		Values: Row{
			"Catalog or Reference Number": "MAT-001",
			"Primary DI":                  "DI-001",
			"Product Name/Generic Name":   "Device name",
		},
	}
	if err := log.AppendRecords([]OperationLogRecord{record}, "material_code"); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	remaining, err := log.DropUnchangedRecords([]OperationLogRecord{record}, "material_code")
	if err != nil {
		t.Fatalf("比较失败：%v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("值一致的记录不该再写一遍：%v", remaining)
	}

	changed := OperationLogRecord{
		Key: "MAT-001",
		Values: Row{
			"Catalog or Reference Number": "MAT-001",
			"Primary DI":                  "DI-001",
			"Product Name/Generic Name":   "Changed device",
		},
	}
	if remaining, err = log.DropUnchangedRecords([]OperationLogRecord{changed}, "material_code"); err != nil {
		t.Fatalf("比较失败：%v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("变化的记录要照常追加：%v", remaining)
	}

	unknown := OperationLogRecord{
		Key: "MAT-002",
		Values: Row{
			"Catalog or Reference Number": "MAT-002",
			"Primary DI":                  "DI-002",
		},
	}
	if remaining, err = log.DropUnchangedRecords([]OperationLogRecord{unknown}, "material_code"); err != nil {
		t.Fatalf("比较失败：%v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("新身份要照常写入：%v", remaining)
	}
}

func TestR22AChangedRecordKeepsItsHistory(t *testing.T) {
	loader := newLoader(t)
	repository := newRepository(t)
	log := newOperationLog(t, repository, loader)
	log.Now = fixedClock("2026/01/01 00:00:00")
	first := OperationLogRecord{
		Key: "MAT-001",
		Values: Row{
			"Catalog or Reference Number": "MAT-001",
			"Primary DI":                  "DI-001",
			"Product Name/Generic Name":   "Device name",
		},
	}
	if err := log.AppendRecords([]OperationLogRecord{first}, "material_code"); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}
	log.Now = fixedClock("2026/02/02 00:00:00")
	second := OperationLogRecord{
		Key: "MAT-001",
		Values: Row{
			"Catalog or Reference Number": "MAT-001",
			"Primary DI":                  "DI-001",
			"Product Name/Generic Name":   "Changed device",
		},
	}
	remaining, err := log.DropUnchangedRecords([]OperationLogRecord{second}, "material_code")
	if err != nil {
		t.Fatalf("比较失败：%v", err)
	}
	if err := log.AppendRecords(remaining, "material_code"); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}

	rows, err := log.ReadByTime("2000/01/01 00:00:00", "2999/12/31 23:59:59")
	if err != nil {
		t.Fatalf("读日志失败：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("变更历史应当保留两条，得到 %d", len(rows))
	}
	if rows[1]["Product Name/Generic Name"] != "Changed device" {
		t.Errorf("最后一条是变更后的值：%v", rows[1])
	}
}
