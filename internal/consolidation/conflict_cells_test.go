package consolidation

import (
	"sort"
	"strings"
	"testing"

	"github.com/jiegun314/ssr-go/internal/store"
)

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

// TestR14ConflictCandidatesNameTheConflictingExportedCellsManyToOne 覆盖 many_to_one：
// 同一 material_code 出现两个不同的医保编码时，冲突的格是「Medical Insurance Code」
// （该来源的 db_field 反查到的唯一导出列；concatenate 的另一个字段取值相同，不并入候选值）。
func TestR14ConflictCandidatesNameTheConflictingCellsManyToOne(t *testing.T) {
	service, _, repository := consolidated(t)
	before := ""
	after := ""
	replaceRows(t, repository, "medical_insurance_code", func(rows []store.Row) []store.Row {
		// 复制第一行并把医保编码改成另一个值：同一 material_code 两个不同值 ⇒ 冲突（R14）
		before = rows[0]["medical_insurance_code"]
		after = before + "-X"
		conflictRow := store.Row{}
		for column, value := range rows[0] {
			conflictRow[column] = value
		}
		conflictRow["medical_insurance_code"] = after
		return append(rows, conflictRow)
	})

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}

	found := conflicts["medical_insurance_code"]
	if len(found.Details) != 1 {
		t.Fatalf("行级冲突 = %v; want 1 条", found.Details)
	}
	if len(found.Candidates) != 1 {
		t.Fatalf("单元格级冲突 = %v; want 1 个键", found.Candidates)
	}
	expected := []string{before, after}
	sort.Strings(expected)
	for key, columns := range found.Candidates {
		if _, present := found.Details[key]; !present {
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
// 提示按**导出列名**归位，文案是「来源 + 键 + 候选值」。
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
	want := "UDI团队信息：" + strings.ReplaceAll(key, "\x00", "+") +
		" 有多个候选值：" + strings.Join(candidates, ", ")
	if note != want {
		t.Errorf("提示 = %q; want %q", note, want)
	}
}

// TestR14ConflictNotesListAtMostThreeCandidates 固定简版口径：候选值只列前 3 条。
func TestR14ConflictNotesListAtMostThreeCandidates(t *testing.T) {
	service, _, repository := consolidated(t)
	base := ""
	materialCode := ""
	replaceRows(t, repository, "medical_insurance_code", func(rows []store.Row) []store.Row {
		base = rows[0]["medical_insurance_code"]
		materialCode = rows[0]["material_code"]
		extra := []store.Row{}
		for _, suffix := range []string{"-1", "-2", "-3"} {
			conflictRow := store.Row{}
			for column, value := range rows[0] {
				conflictRow[column] = value
			}
			conflictRow["medical_insurance_code"] = base + suffix
			extra = append(extra, conflictRow)
		}
		return append(rows, extra...)
	})

	_, conflicts, err := service.readSourceData()
	if err != nil {
		t.Fatalf("读来源数据失败：%v", err)
	}
	key := onlyConflictKey(t, conflicts["medical_insurance_code"].Details)
	if key != materialCode {
		t.Fatalf("many_to_one 的冲突键 = %q; want 基准键 %q", key, materialCode)
	}
	notes := service.conflictNotes(conflictScope{Conflicts: conflicts, BaseKey: materialCode})

	candidates := []string{base, base + "-1", base + "-2", base + "-3"}
	sort.Strings(candidates)
	want := "医保编码信息：" + materialCode + " 有多个候选值：" +
		strings.Join(candidates[:conflictNoteLimit], ", ")
	if notes["Medical Insurance Code"] != want {
		t.Errorf("提示 = %q; want %q", notes["Medical Insurance Code"], want)
	}
	if strings.Contains(notes["Medical Insurance Code"], base+"-3") {
		t.Errorf("超过 3 条的候选值不该列出来：%q", notes["Medical Insurance Code"])
	}
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
// 列顺序 = Config.FieldNames，空串 = 该格不冲突。
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
		for position, text := range notes {
			if row.Values["status"] == StatusConflict && position == column {
				if !strings.Contains(text, strings.Join(candidates, ", ")) {
					t.Errorf("第 %d 行 %q 的提示 = %q; want 含 %q",
						index, service.Config.FieldNames[position], text, strings.Join(candidates, ", "))
				}
				marked++
				continue
			}
			if text != "" {
				t.Errorf("第 %d 行 %q 不该有提示：%q",
					index, service.Config.FieldNames[position], text)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("被标记的冲突格 = %d; want 1", marked)
	}
}

// TestR14AConflictFreeRunHasAnEmptyNotesMatrix：样本本身没有冲突（现有场景 Conflict = 0），
// 矩阵仍然与 Rows 同形同序，但每一格都是空串 —— 结果与 golden 基线都不受影响。
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
		for position, text := range notes {
			if text != "" {
				t.Errorf("没有冲突时第 %d 行 %q 不该有提示：%q",
					index, service.Config.FieldNames[position], text)
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
