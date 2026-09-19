package consolidation

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/store"
	"github.com/jiegun314/ssr-go/internal/validation"
)

// consolidated 导入四份样本并跑一次整合，返回服务、结果与仓库。
func consolidated(t *testing.T) (*Service, Outcome, *store.Repository) {
	t.Helper()
	loader, repository, importService := newWorkspace(t)
	for _, sourceName := range importService.Order {
		path := filepath.Join("..", "..", "testdata", "sample-valid", sourceName+".xlsx")
		if _, err := importService.Import(sourceName, path); err != nil {
			t.Fatalf("导入 %s 失败：%v", sourceName, err)
		}
	}
	service := newService(t, loader, repository)
	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}
	return service, outcome, repository
}

// recordReady 把 Ready 行写进操作日志（等价于导出成功后的 record_consolidation_result）。
func recordReady(t *testing.T, service *Service, outcome Outcome) {
	t.Helper()
	records := []store.OperationLogRecord{}
	for _, row := range outcome.Rows {
		if row.Values["status"] != StatusReady {
			continue
		}
		records = append(records, store.OperationLogRecord{
			Key:    row.Values[service.Config.BaseKeyField],
			Values: row.Values,
		})
	}
	if err := service.Log.AppendRecords(records, service.Config.BaseKeyField); err != nil {
		t.Fatalf("写日志失败：%v", err)
	}
}

func firstReadyIdentity(outcome Outcome) string {
	for _, row := range outcome.Rows {
		if row.Values["status"] == StatusReady {
			return row.Key
		}
	}
	return ""
}

func TestR17ARecordedResultBecomesDuplicateOnTheNextRun(t *testing.T) {
	service, outcome, _ := consolidated(t)
	first := firstReadyIdentity(outcome)
	recordReady(t, service, outcome)

	second, err := service.Consolidate()
	if err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}

	counts := map[string]int{}
	for _, row := range second.Rows {
		counts[row.Values["status"]]++
	}
	if counts[StatusDuplicate] != 8 || counts[StatusReady] != 0 || counts[StatusIncomplete] != 1 {
		t.Fatalf("第二次整合的状态计数 = %v", counts)
	}
	if len(second.DuplicateData) != 8 {
		t.Fatalf("重复明细 = %v", second.DuplicateData)
	}
	if len(second.ChangedData) != 0 {
		t.Fatalf("没有变化就不该有变更明细：%v", second.ChangedData)
	}
	// 描述列在未变化时取 on_no_change（当前配置为空）
	for _, row := range second.Rows {
		if row.Key == first && row.Values["Change Description"] != "" {
			t.Errorf("未变化的行描述应为空：%q", row.Values["Change Description"])
		}
	}
}

func TestR18AChangedColumnIsDescribedAndStaysExportable(t *testing.T) {
	service, outcome, repository := consolidated(t)
	first := firstReadyIdentity(outcome)
	recordReady(t, service, outcome)
	// 改掉 RA 里的产品名称
	changeSourceValue(t, repository, "ra_input_staging", "000MAT-001", "product_name", "Changed device")

	second, err := service.Consolidate()
	if err != nil {
		t.Fatalf("第二次整合失败：%v", err)
	}

	affected := []string{}
	for _, row := range second.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		affected = append(affected, row.Key)
		if row.Values["status"] != StatusReady {
			t.Errorf("%s 状态 = %q; want Ready", row.Key, row.Values["status"])
		}
		if row.Values["Change Description"] != "Product Name/Generic Name changed" {
			t.Errorf("%s 描述 = %q", row.Key, row.Values["Change Description"])
		}
	}
	if len(affected) == 0 || strings.Join(second.ChangedData, "|") != strings.Join(affected, "|") {
		t.Fatalf("变更明细 = %v; want %v", second.ChangedData, affected)
	}
	if second.Rows[0].Key != first {
		t.Errorf("第一条结果的身份不该变：%q", second.Rows[0].Key)
	}
}

func TestR14DifferentRowsOfTheSameCodeAndDiAreAConflict(t *testing.T) {
	service, _, repository := consolidated(t)
	replaceRows(t, repository, "global_udi_input_staging", func(rows []store.Row) []store.Row {
		// 复制第一行并改掉数量：同 material_code + DI 的两行内容不同 ⇒ 冲突（R14）
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["quantity_per_min_sales_unit"] = "999"
		return append(rows, conflictRow)
	})

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}

	conflicts := 0
	for _, row := range outcome.Rows {
		if row.Values["status"] == StatusConflict {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("冲突行数 = %d; want 1", conflicts)
	}
	if len(outcome.ConflictData) != 1 ||
		!strings.Contains(outcome.ConflictData[0], "UDI团队信息") {
		t.Fatalf("冲突文案 = %v", outcome.ConflictData)
	}
}

func TestR13AMissingInsuranceRowKeepsTheRecordExportable(t *testing.T) {
	service, _, repository := consolidated(t)
	changeSourceValue(t, repository, "medical_insurance_code", "000MAT-001",
		"material_code", "OTHER-MAT")

	outcome, err := service.Consolidate()
	if err != nil {
		t.Fatalf("整合失败：%v", err)
	}

	for _, row := range outcome.Rows {
		if row.Values[service.Config.BaseKeyField] != "000MAT-001" {
			continue
		}
		if row.Values["Medical Insurance Code"] != "" {
			t.Errorf("没有医保行时该列为空：%q", row.Values["Medical Insurance Code"])
		}
		if row.Values["status"] == StatusIncomplete {
			t.Errorf("医保来源声明了 no_match: empty，状态不该是 Incomplete")
		}
	}
	if len(outcome.MissingData) != 1 || outcome.MissingData[0] != "000MAT-006::MISSING" {
		t.Fatalf("缺失明细 = %v", outcome.MissingData)
	}
}

func TestR10AMissingSourceTableStopsTheConsolidation(t *testing.T) {
	service, _, repository := consolidated(t)
	if err := repository.DropTable("global_udi_input_staging"); err != nil {
		t.Fatalf("删表失败：%v", err)
	}

	_, err := service.Consolidate()

	var missing *validation.MissingSourceDataError
	if !errors.As(err, &missing) {
		t.Fatalf("缺表时必须停下并报来源（R10），得到：%v", err)
	}
	if !strings.Contains(err.Error(), "数据整合未执行") ||
		!strings.Contains(err.Error(), "global_udi_input（UDI团队信息，表：global_udi_input_staging不存在）") {
		t.Fatalf("缺表文案不对：%s", err.Error())
	}
}

func TestR10AnEmptySourceTableStopsTheConsolidation(t *testing.T) {
	service, _, repository := consolidated(t)
	// 把医保编码表清空（表还在，但没有记录）
	table := service.Config.SourceTables["medical_insurance_code"]
	if err := repository.WriteRows(table, []string{"material_code"}, nil, store.WriteReplace); err != nil {
		t.Fatalf("清空表失败：%v", err)
	}

	_, err := service.Consolidate()

	if err == nil || !strings.Contains(err.Error(),
		"medical_insurance_code（医保编码信息，表：medical_insurance_code）") {
		t.Fatalf("空表文案不对：%v", err)
	}
}

// changeSourceValue 改一张来源表里某个键对应的一个单元格。
func changeSourceValue(
	t *testing.T,
	repository *store.Repository,
	tableName string,
	matchValue string,
	column string,
	value string,
) {
	t.Helper()
	changed := false
	replaceRows(t, repository, tableName, func(rows []store.Row) []store.Row {
		for index := range rows {
			if rows[index]["material_code"] == matchValue {
				rows[index][column] = value
				changed = true
			}
		}
		return rows
	})
	if !changed {
		t.Fatalf("表 %s 里没有 %s", tableName, matchValue)
	}
}

// replaceRows 读出整张表、按 mutate 改写后再写回去（列顺序不变）。
func replaceRows(
	t *testing.T,
	repository *store.Repository,
	tableName string,
	mutate func(rows []store.Row) []store.Row,
) {
	t.Helper()
	columns, err := repository.TableColumns(tableName)
	if err != nil {
		t.Fatalf("读列失败：%v", err)
	}
	rows, err := repository.Rows(tableName)
	if err != nil {
		t.Fatalf("读行失败：%v", err)
	}
	if err := repository.WriteRows(
		tableName, columns, mutate(rows), store.WriteReplace); err != nil {
		t.Fatalf("写回失败：%v", err)
	}
}
