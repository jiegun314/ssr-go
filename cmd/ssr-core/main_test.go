package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestTheCLIListsEveryDocumentedSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--help 应返回 0，得到 %d", code)
	}
	for _, name := range []string{
		"version", "import", "consolidate", "export", "snapshot",
		"genlogcolumns", "alignlogcolumns", "gensample", "preparedb",
	} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("用法里缺少子命令 %s", name)
		}
	}
}

func TestAnUnknownSubcommandFailsWithUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"nonsense"}, &stdout, &stderr); code != 2 {
		t.Fatalf("未知子命令应返回 2，得到 %d", code)
	}
	if !strings.Contains(stderr.String(), "未知子命令") {
		t.Errorf("错误输出缺少提示：%q", stderr.String())
	}
}

func TestVersionReportsAVersionString(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version 应返回 0，得到 %d（stderr: %s）", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "版本：") {
		t.Errorf("version 输出不对：%q", stdout.String())
	}
}

func TestAPortedSubcommandSaysItIsNotImplementedYet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"alignlogcolumns"}, &stdout, &stderr); code != 2 {
		t.Fatalf("尚未移植的子命令应返回 2，得到 %d", code)
	}
	if !strings.Contains(stderr.String(), "尚未实现") {
		t.Errorf("应明确说明尚未实现：%q", stderr.String())
	}
}
