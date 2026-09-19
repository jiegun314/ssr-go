package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportMatchesThePythonBaseline 是导入阶段的 golden diff。
//
// 对照物是 Python 现状实现产出的 `baseline/来源表_<来源>.tsv`（AGENTS.md §9.3 的补充
// 证据）：同一批样本工作簿导入后，暂存表的**列顺序、行顺序与每个单元格的文本**都必须
// 与现状实现逐格一致。这是「数据口径①」在导入环节的落地形式。
func TestImportMatchesThePythonBaseline(t *testing.T) {
	importer, repository, _ := newTestImporter(t)

	for _, sourceName := range importer.Order {
		t.Run(sourceName, func(t *testing.T) {
			rule := importer.Rules[sourceName]
			if _, err := importer.Import(sourceName, samplePath(sourceName)); err != nil {
				t.Fatalf("导入 %s 失败：%v", sourceName, err)
			}
			golden := readTSV(t, filepath.Join(
				"..", "..", "testdata", "baseline", "来源表_"+sourceName+".tsv"))
			if len(golden) == 0 {
				t.Fatalf("%s 的 baseline 是空的", sourceName)
			}
			configColumns := dbFields(rule)
			if strings.Join(golden[0], "|") != strings.Join(configColumns, "|") {
				t.Fatalf("baseline 的列顺序与配置不一致 baseline=%v config=%v",
					golden[0], configColumns)
			}
			rows, err := repository.Rows(rule.TargetTable)
			if err != nil {
				t.Fatalf("读暂存表失败：%v", err)
			}
			if len(rows) != len(golden)-1 {
				t.Fatalf("行数：Go = %d，baseline = %d", len(rows), len(golden)-1)
			}
			for position, goldenRow := range golden[1:] {
				for index, column := range golden[0] {
					wanted := unescapeTSVCell(goldenRow[index])
					got := rows[position][column]
					if got != wanted {
						t.Errorf("第 %d 行 %q：Go = %q，baseline = %q",
							position+1, column, got, wanted)
					}
				}
			}
		})
	}
}

// readTSV 读一个 TSV 文件，返回所有行（第一行是表头）。
func readTSV(t *testing.T, path string) [][]string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 baseline 失败：%v", err)
	}
	text := strings.TrimRight(string(content), "\n")
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

// unescapeTSVCell 还原基线里的转义（写快照时 \\ \t \r \n 都被转义过）。
func unescapeTSVCell(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character != '\\' || index+1 >= len(value) {
			builder.WriteByte(character)
			continue
		}
		index++
		switch value[index] {
		case '\\':
			builder.WriteByte('\\')
		case 't':
			builder.WriteByte('\t')
		case 'r':
			builder.WriteByte('\r')
		case 'n':
			builder.WriteByte('\n')
		default:
			builder.WriteByte(value[index])
		}
	}
	return builder.String()
}
