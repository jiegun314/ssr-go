package app

// P6：诊断包（试点期排查用）。把版本、日志、审计、共享盘状态与脱敏后的配置打成 zip。
//
// 口径：
//   - 不含本地数据库（体积大且含业务数据）；
//   - 配置文件里的邮箱一律打码（***@***），其余原样；
//   - 输出到程序目录下的 output/diagnostics-<时间>.zip。

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/buildinfo"
	"github.com/jiegun314/ssr-go/internal/config"
)

// DiagnosticsResult 是一次诊断包导出的结果。
type DiagnosticsResult struct {
	Failed  bool     `json:"failed"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Log     string   `json:"log"`
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
}

var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+`)

// maskEmails 把邮箱打码（诊断包只用于排查，不需要真实地址）。
func maskEmails(text string) string {
	return emailPattern.ReplaceAllString(text, "***@***")
}

// ExportDiagnostics 生成诊断包（SC/Admin）。
func (app *App) ExportDiagnostics() DiagnosticsResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionViewAuditAny); err != nil {
		return DiagnosticsResult{Failed: true, Title: "无权限", Message: userMessage(err), Log: app.logText()}
	}
	if app.loader == nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: "配置未加载，无法生成诊断包。", Log: app.logText()}
	}
	outputDir := filepath.Join(filepath.Dir(app.loader.Resolver.ConfigDir), "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	path := filepath.Join(outputDir, "diagnostics-"+time.Now().Format("20060102T150405")+".zip")
	file, err := os.Create(path)
	if err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	entries := []string{}
	write := func(name string, content []byte) error {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(content); err != nil {
			return err
		}
		entries = append(entries, name)
		return nil
	}

	// 版本与共享盘状态
	version := buildinfo.ReadBuildInfo(buildinfo.Options{})
	if err := write("version.txt", []byte(version.Detail()+"\n")); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	statusJSON, _ := json.MarshalIndent(map[string]any{
		"mode": app.shareStatus.Mode, "healthy": app.shareStatus.Healthy,
		"detail": app.shareStatus.Detail, "checked_at": app.shareStatus.CheckedAt,
		"share_root": app.shareConfig.Root, "on_new_version": app.shareConfig.OnNewVersion,
	}, "", "  ")
	if err := write("share-status.json", append(statusJSON, '\n')); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	// 共享盘状态文件（有就带一份）
	if app.shareEnabled() {
		if content, readErr := os.ReadFile(filepath.Join(app.shareConfig.Root, "state", "state.json")); readErr == nil {
			if err := write("share-state.json", content); err != nil {
				return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
			}
		}
	}
	// 运行日志（界面右下角的那些行）
	if err := write("operation-log.txt", []byte(app.logText())); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	// 审计
	if rows, auditErr := app.repo.AuditRows(); auditErr == nil {
		var buffer bytes.Buffer
		csvWriter := csv.NewWriter(&buffer)
		_ = csvWriter.Write(AuditColumns)
		for _, row := range rows {
			_ = csvWriter.Write([]string{
				row["at"], row["actor"], row["role"], row["username"], row["machine"],
				row["action"], row["target"], row["detail"], row["result"],
			})
		}
		csvWriter.Flush()
		if err := write("audit.csv", buffer.Bytes()); err != nil {
			return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
		}
	}
	// 配置（邮箱打码）
	configFiles := []string{
		config.SettingFile, config.ImportMappingFile, config.ConsolidationMappingFile,
		config.LogColumnsFile, config.WorkflowFile, config.UsersFile,
	}
	for _, name := range configFiles {
		content, readErr := os.ReadFile(filepath.Join(app.loader.Resolver.ConfigDir, name))
		if readErr != nil {
			continue
		}
		if err := write("config/"+name, []byte(maskEmails(string(content)))); err != nil {
			return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
		}
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"generated_at": auth.Now(), "generated_by": app.owner(), "entries": entries,
	}, "", "  ")
	if err := write("manifest.json", append(manifest, '\n')); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	if err := writer.Close(); err != nil {
		return DiagnosticsResult{Failed: true, Title: "错误", Message: userMessage(err), Log: app.logText()}
	}
	app.audit("diagnostics", path, fmt.Sprintf("%d 个条目", len(entries)), "ok")
	app.appendLog(LogSuccess, "诊断包已导出："+path)
	return DiagnosticsResult{
		Log: app.logText(), Title: "诊断包已导出",
		Message: "诊断包已导出（不含数据库，邮箱已打码）：\n" + path,
		Path:    path, Entries: entries,
	}
}
