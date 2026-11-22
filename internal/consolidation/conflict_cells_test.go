package consolidation

import (
	"sort"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/store"
)

func candidatesString(candidates []string) string {
	return strings.Join(candidates, ",")
}

// 这些用例覆盖 AGENTS.md §11 第 1 步：收口时除行级冲突文案外，
// 还要产出「来源键 → 导出列名 → 候选值集合」，只标真正冲突的格。

// TestR14ConflictCandidatesNameTheConflictingExportedCells 覆盖 one_to_many：
// 同一 material_code + DI 的两行内容不同（R14 的行级冲突）时，
// 候选值必须落在**值真的不一样**的导出列上，值相同的列（这里是匹配键 DI）不标。
func TestR14ConflictCandidatesNameTheConflictingExportedCells(t *testing.T) {
	service, _, repository := consolidated(t)
	original := ""
	replaceRows(t, repository, "global_udi_input_staging", func(rows []store.Row) []store.Row {
		// 复制第一行并改掉数量：两行只有这一列不同
		original = rows[0]["quantity_per_min_sales_unit"]
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["quantity_per_min_sales_unit"] = "999"
		return append(rows, conflictRow)
	})

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}

	found := conflicts["global_udi_input"]
	if len(found.Details) != 1 {
		t.Fatalf("行级冲突 = %v; want 1 条", found.Details)
	}
	if len(found.Candidates) != 1 {
		t.Fatalf("单元格级冲突 = %v; want 1 个键", found.Candidates)
	}
	expected := []string{original, "999"}
	sort.Strings(expected)
	for key, columns := range found.Candidates {
		if _, present := found.Details[key]; !present {
			t.Errorf("候选值的键 %q 不在行级冲突里", key)
		}
		if len(columns) != 1 ||
			strings.Join(columns["Quantity of Device"], ",") != strings.Join(expected, ",") {
			t.Errorf("键 %q 的候选值 = %v; want Quantity of Device = %v",
				key, columns, expected)
		}
		if _, present := columns["Primary DI"]; present {
			t.Errorf("两行取值相同的列不是冲突的格，不该有候选值：%v", columns)
		}
	}
}

// TestR14ConflictCandidatesNameTheConflictingCellsManyToOne 覆盖 many_to_one 的冲突口径：
// 同一 material_code 出现两个不同的医保编码时，冲突的格是「Medical Insurance Code」
// （该来源的 db_field 反查到的唯一导出列）。
//
// 注意：仓库配置现在让医保来源走 merge_records（同一型号多条编码合并成一格、不判冲突），
// 所以这里用一条等价的手写规则直接喂给 groupManyToOne，保留 many_to_one 冲突路径的覆盖。
func TestR14ConflictCandidatesNameTheConflictingCellsManyToOne(t *testing.T) {
	service, _, repository := consolidated(t)
	rows := insuranceRowsWithCodes(t, repository, service, "-X")

	_, details, candidates, _ := service.groupManyToOne(
		"medical_insurance_code", rows, manyToOneConflictRule())

	if len(details) != 1 {
		t.Fatalf("行级冲突 = %v; want 1 条", details)
	}
	if len(candidates) != 1 {
		t.Fatalf("单元格级冲突 = %v; want 1 个键", candidates)
	}
	expected := []string{rows[0]["medical_insurance_code"], rows[1]["medical_insurance_code"]}
	sort.Strings(expected)
	for key, columns := range candidates {
		if _, present := details[key]; !present {
			t.Errorf("候选值的键 %q 不在行级冲突里", key)
		}
		if len(columns) != 1 ||
			strings.Join(columns["Medical Insurance Code"], ",") != strings.Join(expected, ",") {
			t.Errorf("键 %q 的候选值 = %v; want Medical Insurance Code = %v",
				key, columns, expected)
		}
	}
}

// TestR14AConflictWithoutAnExportedColumnMarksNoCell：两行只在**没有导出列**的字段上
// 不同（franchise 只存在于暂存表，没有喂给任何导出列）时，行级冲突照记，
// 但没有可标记的单元格，因此候选值里没有这个键。
func TestR14AConflictWithoutAnExportedColumnMarksNoCell(t *testing.T) {
	service, _, repository := consolidated(t)
	replaceRows(t, repository, "global_udi_input_staging", func(rows []store.Row) []store.Row {
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["franchise"] = conflictRow["franchise"] + "-X"
		return append(rows, conflictRow)
	})

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}

	found := conflicts["global_udi_input"]
	if len(found.Details) != 1 {
		t.Fatalf("行级冲突 = %v; want 1 条", found.Details)
	}
	if len(found.Candidates) != 0 {
		t.Errorf("没有导出列冲突时不该有候选值：%v", found.Candidates)
	}
}

// TestR14AConflictFreeRunMarksNoCells 是反向保证：样本本身没有冲突，
// 收口就不该标出任何单元格（现有场景 Conflict = 0，结果也不能变）。
func TestR14AConflictFreeRunMarksNoCells(t *testing.T) {
	service, _, _ := consolidated(t)

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}

	for sourceName, found := range conflicts {
		if len(found.Details) != 0 || len(found.Candidates) != 0 {
			t.Errorf("%s 没有冲突时不该有冲突信息：%+v", sourceName, found)
		}
	}
}

// --- 第 2 步：单元格级提示文案 ---

// TestR14ConflictNotesLandOnTheConflictingColumn 覆盖 one_to_many 的提示文案：
// 提示按**导出列名**归位，结构化数据里包含来源、键、列名和候选值。
func TestR14ConflictNotesLandOnTheConflictingColumn(t *testing.T) {
	service, _, repository := consolidated(t)
	original := ""
	replaceRows(t, repository, "global_udi_input_staging", func(rows []store.Row) []store.Row {
		original = rows[0]["quantity_per_min_sales_unit"]
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["quantity_per_min_sales_unit"] = "999"
		return append(rows, conflictRow)
	})

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}
	key := onlyConflictKey(t, conflicts["global_udi_input"].Details)
	notes := service.conflictNotes(scopeForConflictKey(t, conflicts, "global_udi_input", key))

	note, present := notes["Quantity of Device"]
	if !present || len(notes) != 1 {
		t.Fatalf("提示 = %v; want 只有 Quantity of Device", notes)
	}
	candidates := []string{original, "999"}
	sort.Strings(candidates)
	if note.Source != "UDI团队信息" {
		t.Errorf("来源 = %q; want UDI团队信息", note.Source)
	}
	if note.Key != strings.ReplaceAll(key, "\x00", "+") {
		t.Errorf("键 = %q; want %q", note.Key, strings.ReplaceAll(key, "\x00", "+"))
	}
	if note.Column != "Quantity of Device" {
		t.Errorf("列 = %q; want Quantity of Device", note.Column)
	}
	if candidatesString(note.Candidates) != candidatesString(candidates) {
		t.Errorf("候选值 = %v; want %v", note.Candidates, candidates)
	}
}

// TestR14ConflictNotesListAtMostThreeCandidates 固定口径：候选值最多只列前 3 条。
//
// 同样用手写的 many_to_one 冲突规则（仓库配置对医保来源已改成 merge_records）。
func TestR14ConflictNotesListAtMostThreeCandidates(t *testing.T) {
	service, _, repository := consolidated(t)
	rows := insuranceRowsWithCodes(t, repository, service, "-1", "-2", "-3")
	materialCode := rows[0]["material_code"]
	base := rows[0]["medical_insurance_code"]

	_, details, candidates, _ := service.groupManyToOne(
		"medical_insurance_code", rows, manyToOneConflictRule())
	key := onlyConflictKey(t, details)
	if key != materialCode {
		t.Fatalf("many_to_one 的冲突键 = %q; want 基准键 %q", key, materialCode)
	}
	conflicts := map[string]sourceConflicts{
		"medical_insurance_code": {Details: details, Candidates: candidates},
	}
	notes := service.conflictNotes(conflictScope{Conflicts: conflicts, BaseKey: materialCode})

	all := []string{base, base + "-1", base + "-2", base + "-3"}
	sort.Strings(all)
	note := notes["Medical Insurance Code"]
	if note == nil {
		t.Fatal("Medical Insurance Code 应当有冲突提示")
	}
	if note.Source != "医保编码信息" {
		t.Errorf("来源 = %q; want 医保编码信息", note.Source)
	}
	if candidatesString(note.Candidates) != candidatesString(all[:conflictNoteLimit]) {
		t.Errorf("候选值 = %v; want %v", note.Candidates, all[:conflictNoteLimit])
	}
	if contains(note.Candidates, base+"-3") {
		t.Errorf("超过 3 条的候选值不该列出来：%v", note.Candidates)
	}
}

// manyToOneConflictRule 是医保来源改配置之前的等价规则：取第一条、不同值算冲突。
// 只用来覆盖 many_to_one 的冲突路径，不代表仓库现在的配置。
func manyToOneConflictRule() map[string]any {
	return map[string]any{
		"cardinality": "many_to_one",
		"match":       map[string]any{"material_code": "material_code"},
		"duplicates": map[string]any{
			"compare_by":      []any{"medical_insurance_code"},
			"different_value": "conflict",
			"selected_record": "first",
		},
	}
}

// insuranceRowsWithCodes 取医保来源的第一条真实行，再按 suffixes 造出同一个 material_code 的
// 多条不同编码（编码 = 原编码 + suffix）。返回 [原行, 原行+suffix1, …]，保持来源顺序。
func insuranceRowsWithCodes(
	t *testing.T,
	repository *store.Repository,
	service *Service,
	suffixes ...string,
) []store.Row {
	t.Helper()
	rows, err := repository.Rows(service.Config.SourceTables["medical_insurance_code"])
	if err != nil {
		t.Fatalf("读医保来源失败：%v", err)
	}
	if len(rows) == 0 {
		t.Fatal("医保来源没有数据")
	}
	original := rows[0]
	extra := make([]store.Row, 0, len(suffixes))
	for _, suffix := range suffixes {
		conflictRow := store.Row{}
		for column, value := range original {
			conflictRow[column] = value
		}
		conflictRow["medical_insurance_code"] = original["medical_insurance_code"] + suffix
		extra = append(extra, conflictRow)
	}
	return append([]store.Row{original}, extra...)
}

// TestR14NoMatchRowsCarryNoCellNotes：来源没有匹配记录的占位行（no_match）没有冲突格，
// 即便冲突结构里恰好存在它那个键，也不该给出提示。
func TestR14NoMatchRowsCarryNoCellNotes(t *testing.T) {
	service, _, _ := consolidated(t)
	conflicts := map[string]sourceConflicts{
		"global_udi_input": {Candidates: conflictCandidates{
			"000MAT-006\x00MISSING": {"Primary DI": {"A", "B"}},
		}},
	}
	scope := conflictScope{
		Conflicts:       conflicts,
		BaseKey:         "000MAT-006",
		ExpansionSource: "global_udi_input",
		ExpansionKey:    "MISSING",
		ExpansionRow:    nil,
	}

	if notes := service.conflictNotes(scope); len(notes) != 0 {
		t.Errorf("占位行不该有单元格提示：%v", notes)
	}
	if details := service.conflictDetails(scope); len(details) != 0 {
		t.Errorf("占位行不该有行级冲突文案：%v", details)
	}
}

// --- 第 3 步：Outcome.ConflictNotes 矩阵 ---

// TestR14ConflictNotesMatrixAlignsWithRows：ConflictNotes 与 Rows 同形同序，
// 列顺序 = Config.FieldNames，nil = 该格不冲突。
func TestR14ConflictNotesMatrixAlignsWithRows(t *testing.T) {
	service, _, repository := consolidated(t)
	original := ""
	replaceRows(t, repository, "global_udi_input_staging", func(rows []store.Row) []store.Row {
		original = rows[0]["quantity_per_min_sales_unit"]
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

	if len(outcome.ConflictNotes) != len(outcome.Rows) {
		t.Fatalf("ConflictNotes 有 %d 行; want %d（与 Rows 同形同序）",
			len(outcome.ConflictNotes), len(outcome.Rows))
	}
	column := columnIndex(t, service, "Quantity of Device")
	candidates := []string{original, "999"}
	sort.Strings(candidates)
	marked := 0
	for index, row := range outcome.Rows {
		notes := outcome.ConflictNotes[index]
		if len(notes) != len(service.Config.FieldNames) {
			t.Fatalf("第 %d 行的提示有 %d 格; want %d",
				index, len(notes), len(service.Config.FieldNames))
		}
		for position, cell := range notes {
			if row.Values["status"] == StatusConflict && position == column {
				if cell == nil {
					t.Errorf("第 %d 行 %q 的冲突格应当有提示", index, service.Config.FieldNames[position])
				} else if candidatesString(cell.Candidates) != candidatesString(candidates) {
					t.Errorf("第 %d 行 %q 的候选值 = %v; want %v",
						index, service.Config.FieldNames[position], cell.Candidates, candidates)
				}
				marked++
				continue
			}
			if cell != nil {
				t.Errorf("第 %d 行 %q 不该有提示：%+v",
					index, service.Config.FieldNames[position], cell)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("被标记的冲突格 = %d; want 1", marked)
	}
}

// TestR14AConflictFreeRunHasAnEmptyNotesMatrix：样本本身没有冲突（现有场景 Conflict = 0），
// 矩阵仍然与 Rows 同形同序，但每一格都是 nil —— 结果与 golden 基线都不受影响。
func TestR14AConflictFreeRunHasAnEmptyNotesMatrix(t *testing.T) {
	service, outcome, _ := consolidated(t)

	if len(outcome.ConflictNotes) != len(outcome.Rows) {
		t.Fatalf("ConflictNotes 有 %d 行; want %d（与 Rows 同形同序）",
			len(outcome.ConflictNotes), len(outcome.Rows))
	}
	for index, notes := range outcome.ConflictNotes {
		if len(notes) != len(service.Config.FieldNames) {
			t.Fatalf("第 %d 行的提示有 %d 格; want %d",
				index, len(notes), len(service.Config.FieldNames))
		}
		for position, cell := range notes {
			if cell != nil {
				t.Errorf("没有冲突时第 %d 行 %q 不该有提示：%+v",
					index, service.Config.FieldNames[position], cell)
			}
		}
	}
}

// columnIndex 取导出列在 Config.FieldNames 里的位置（ConflictNotes 的列索引就是它）。
func columnIndex(t *testing.T, service *Service, columnName string) int {
	t.Helper()
	for index, name := range service.Config.FieldNames {
		if name == columnName {
			return index
		}
	}
	t.Fatalf("导出列 %q 不在 Config.FieldNames 里", columnName)
	return -1
}

// onlyConflictKey 取唯一的行级冲突键（用例里都只造了一条冲突）。
func onlyConflictKey(t *testing.T, details map[string]string) string {
	t.Helper()
	if len(details) != 1 {
		t.Fatalf("行级冲突 = %v; want 1 条", details)
	}
	for key := range details {
		return key
	}
	return ""
}

// scopeForConflictKey 把收口产出的冲突键还原成结果行的定位：
// one_to_many 的键是「基准键 \x00 拆行键」，拆行来源这一行必须有记录（ExpansionRow 非空）。
func scopeForConflictKey(
	t *testing.T,
	conflicts map[string]sourceConflicts,
	expansionSource string,
	key string,
) conflictScope {
	t.Helper()
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		t.Fatalf("冲突键 %q 不是「基准键+拆行键」", key)
	}
	return conflictScope{
		Conflicts:       conflicts,
		BaseKey:         parts[0],
		ExpansionSource: expansionSource,
		ExpansionKey:    parts[1],
		ExpansionRow:    store.Row{},
	}
}
