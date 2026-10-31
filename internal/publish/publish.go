// Package publish 把"导入后的整张来源表"发布成共享盘上的版本化全量快照。
//
// 设计口径（docs/design/multiuser-workflow.md §4.2/§7.1）：
//   - 全量：每次都写整张表（不是增量）；
//   - 原子：先写 .tmp 再 rename，最后才更新 state.json；
//   - 版本：<时间戳>-<内容 sha256 前 8 位>，同一份内容得到同一个版本；
//   - 留档：原始 Excel 一并复制成 <version>.xlsx（失败只警告，不影响发布）。
package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jiegun314/ssr-go/internal/fileutil"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

// Options 是一次发布需要的全部输入。
type Options struct {
	Root         string
	Source       string
	Columns      []string
	Rows         []store.Row
	CleanedRows  int
	OriginalPath string
	Owner        share.Owner
	AppVersion   string
	Now          time.Time
}

// Snapshot 是发布成功后的结果。
type Snapshot struct {
	Source       string
	Version      string
	CSVPath      string
	ManifestPath string
	SHA256       string
	Rows         int
	CleanedRows  int
	// OriginalWarning 是"原始 Excel 留档失败"的说明（发布本身已成功）。
	OriginalWarning string
}

// Manifest 是 <version>.manifest.json 的内容。
type Manifest struct {
	Source      string      `json:"source"`
	Version     string      `json:"version"`
	Format      string      `json:"format"`
	Columns     []string    `json:"columns"`
	Rows        int         `json:"rows"`
	CleanedRows int         `json:"cleaned_rows,omitempty"`
	SHA256      string      `json:"sha256"`
	AppVersion  string      `json:"app_version"`
	PublishedBy share.Owner `json:"published_by"`
	PublishedAt string      `json:"published_at"`
}

// Publish 写快照并更新 state.json（返回的 Snapshot 含版本号与路径）。
func Publish(options Options) (Snapshot, error) {
	if options.Root == "" {
		return Snapshot{}, fmt.Errorf("没有配置共享盘（workflow.yaml 的 share.root）")
	}
	if options.Source == "" || len(options.Columns) == 0 {
		return Snapshot{}, fmt.Errorf("发布参数不完整（来源/列）")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	// 1) 整张表写成 CSV（列 = db_field，行 = 归一化后的值）
	content, err := renderCSV(options.Columns, options.Rows)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	version := now.Format("20060102T150405") + "-" + digest[:8]

	directory := filepath.Join(options.Root, "published", share.Department(options.Source))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Snapshot{}, fmt.Errorf("创建发布目录失败 %s：%w", directory, err)
	}
	csvPath := filepath.Join(directory, version+".csv")
	if err := writeAtomic(csvPath, content); err != nil {
		return Snapshot{}, err
	}
	manifest := Manifest{
		Source:      options.Source,
		Version:     version,
		Format:      "csv",
		Columns:     options.Columns,
		Rows:        len(options.Rows),
		CleanedRows: options.CleanedRows,
		SHA256:      digest,
		AppVersion:  options.AppVersion,
		PublishedBy: options.Owner,
		PublishedAt: now.Format("2006-01-02 15:04:05"),
	}
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("序列化 manifest 失败：%w", err)
	}
	manifestPath := filepath.Join(directory, version+".manifest.json")
	if err := writeAtomic(manifestPath, append(manifestContent, '\n')); err != nil {
		return Snapshot{}, err
	}

	// 2) 原始 Excel 留档（失败只警告：发布的是 CSV，留档是审计便利）
	originalWarning := ""
	if options.OriginalPath != "" {
		if err := fileutil.CopyFile(options.OriginalPath, filepath.Join(directory, version+".xlsx")); err != nil {
			originalWarning = "原始文件留档失败：" + err.Error()
		}
	}

	// 3) 最后更新 state.json（先文件后状态；拿不到锁就报错，让用户重试）
	err = share.WithStateLock(options.Root, func() error {
		state, err := share.LoadState(options.Root)
		if err != nil {
			return err
		}
		state.Sources[options.Source] = &share.SourceVersion{
			Version:     version,
			SHA256:      digest,
			Rows:        len(options.Rows),
			CleanedRows: options.CleanedRows,
			Columns:     options.Columns,
			PublishedAt: manifest.PublishedAt,
			PublishedBy: options.Owner,
		}
		state.Generation++
		return state.Save(options.Root)
	})
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Source: options.Source, Version: version, CSVPath: csvPath, ManifestPath: manifestPath,
		SHA256: digest, Rows: len(options.Rows), CleanedRows: options.CleanedRows,
		OriginalWarning: originalWarning,
	}, nil
}

// renderCSV 把整张表渲染成 CSV（固定列顺序、\n 换行 —— sha256 依赖字节稳定）。
func renderCSV(columns []string, rows []store.Row) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write(columns); err != nil {
		return nil, fmt.Errorf("写 CSV 表头失败：%w", err)
	}
	record := make([]string, len(columns))
	for _, row := range rows {
		for index, column := range columns {
			record[index] = row[column]
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("写 CSV 数据行失败：%w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("写 CSV 失败：%w", err)
	}
	return buffer.Bytes(), nil
}

// writeAtomic 先写临时文件再 rename（网络盘上避免读到半个文件）。
func writeAtomic(path string, content []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, content, 0o644); err != nil {
		return fmt.Errorf("写临时文件失败 %s：%w", temp, err)
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return fmt.Errorf("原子写入失败 %s：%w", path, err)
	}
	return nil
}
