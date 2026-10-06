package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestUserMessageTurnsInternalErrorsIntoChinese 固定"界面边界的错误文案"口径：
// 认识的内部错误（英文）给中文说法；不认识的用中文起头并附原文；
// 已经含中文的错误（例如必填缺失明细）原样显示；nil 给空串。
func TestUserMessageTurnsInternalErrorsIntoChinese(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		contains []string
		absent   []string
	}{
		{
			name:     "文件类型不支持",
			err:      fmt.Errorf("Invalid file type: %s. Must be one of %v", ".csv", []string{".xlsx", ".xls"}),
			contains: []string{"文件类型不支持", ".csv", "Excel 工作簿"},
			absent:   []string{"Invalid file type"},
		},
		{
			name:     "文件不存在",
			err:      errors.New("open /Users/x/RA信息.xlsx: no such file or directory"),
			contains: []string{"文件不存在或已被移动"},
			absent:   []string{"no such file"},
		},
		{
			name:     "没有权限",
			err:      errors.New("open /Volumes/Share/out.xlsx: permission denied"),
			contains: []string{"没有权限读写该文件"},
			absent:   []string{"permission denied"},
		},
		{
			name:     "数据库被占用",
			err:      errors.New("database is locked (5) (SQLITE_BUSY)"),
			contains: []string{"数据库被占用"},
			absent:   []string{"SQLITE_BUSY"},
		},
		{
			name:     "不是有效的 Excel",
			err:      errors.New("zip: not a valid zip file"),
			contains: []string{"不是有效的 Excel 工作簿"},
			absent:   []string{"zip"},
		},
		{
			name:     "配置取值不支持",
			err:      errors.New("Unsupported match_type for ra_input: join"),
			contains: []string{"配置里", "match_type", "不支持"},
			absent:   []string{"Unsupported"},
		},
		{
			name:     "配置字段类型不对",
			err:      errors.New("Header row must be a number"),
			contains: []string{"配置项", "number"},
			absent:   []string{"must be a"},
		},
		{
			name:     "导入包装错误（内部原因不具体时给通用说法）",
			err:      errors.New("Failed to import Excel file: unexpected end of file"),
			contains: []string{"导入 Excel 失败"},
			absent:   []string{"Failed to import", "unexpected end"},
		},
		{
			name:     "规则优先级：具体规则排在包装规则之前",
			err:      errors.New("Failed to import Excel file: sheet DEVICE does not exist"),
			contains: []string{"工作簿里找不到需要的工作表"},
			absent:   []string{"Failed to import", "导入 Excel 失败"},
		},
		{
			name:     "不认识且是英文 → 中文起头 + 原文",
			err:      errors.New("something totally unexpected"),
			contains: []string{"操作失败：", "something totally unexpected"},
		},
		{
			name:     "已经含中文 → 原样显示",
			err:      errors.New("导入失败：RA信息\n- 空值：统一社会信用代码（第 13 行）"),
			contains: []string{"导入失败：RA信息", "第 13 行"},
			absent:   []string{"操作失败："},
		},
	}

	for _, item := range cases {
		got := userMessage(item.err)
		for _, wanted := range item.contains {
			if !strings.Contains(got, wanted) {
				t.Errorf("%s：结果里应当含 %q，实际 %q", item.name, wanted, got)
			}
		}
		for _, unwanted := range item.absent {
			if strings.Contains(got, unwanted) {
				t.Errorf("%s：结果里不该再出现英文原文片段 %q，实际 %q", item.name, unwanted, got)
			}
		}
	}

	if userMessage(nil) != "" {
		t.Error("nil 错误应当给空串")
	}
}

// TestEveryUserFacingErrorMessageGoesThroughUserMessage 防止有人再直接把 err.Error()
// 当对话框文案：那些位置必须走 userMessage（日志行里保留原文，属于技术记录）。
func TestEveryUserFacingErrorMessageGoesThroughUserMessage(t *testing.T) {
	source := readSourceFile(t, "app_methods.go")
	if strings.Contains(source, "Message: err.Error()") {
		t.Error("对话框文案应当走 userMessage(err)，不要直接把 err.Error() 交给界面")
	}
	if !strings.Contains(source, "Message: userMessage(err)") {
		t.Error("没有找到经过 userMessage 的对话框文案")
	}
	// 日志行里保留英文原文（便于检索与对照代码）
	if !strings.Contains(source, `appendLog(LogError, "导出失败："+err.Error())`) {
		t.Error("日志里应当保留原始错误文本")
	}
}
