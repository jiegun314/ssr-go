package oplogsync

// 操作日志（「记录导出」里的整合记录）在共享盘上的全量同步。
//
// 所有人都能看"记录导出"，所以这份日志必须同步成同一份全量数据：
//   - 共享盘上 <root>/log/operation_log.csv + operation_log.manifest.json；
//   - 合并口径：按整行内容去重（日志只追加、不改写），共享盘已有的行原样保留；
//   - 发布与合并都在 state.lock 里做（调用方负责加锁）。

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest 是共享盘日志快照的校验信息。
type Manifest struct {
	Rows      int    `json:"rows"`
	SHA256    string `json:"sha256"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

// Snapshot 是拉下来的一份日志快照。
type Snapshot struct {
	Columns  []string
	Rows     [][]string
	Manifest Manifest
}

// Paths 返回共享盘上日志的两个文件。
func Paths(root string) (csvPath string, manifestPath string) {
	directory := filepath.Join(root, "log")
	return filepath.Join(directory, "operation_log.csv"),
		filepath.Join(directory, "operation_log.manifest.json")
}

// Pull 读共享盘上的日志（不存在时返回 os.ErrNotExist）。
func Pull(root string) (Snapshot, error) {
	csvPath, manifestPath := Paths(root)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Snapshot{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Snapshot{}, fmt.Errorf("共享盘操作日志 manifest 损坏：%w", err)
	}
	content, err := os.ReadFile(csvPath)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(content)
	if manifest.SHA256 != "" && hex.EncodeToString(sum[:]) != manifest.SHA256 {
		return Snapshot{}, fmt.Errorf("共享盘操作日志校验失败（sha256 不符）")
	}
	reader := csv.NewReader(bytes.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		return Snapshot{}, fmt.Errorf("共享盘操作日志 CSV 不合法：%w", err)
	}
	if len(records) == 0 {
		return Snapshot{Manifest: manifest}, nil
	}
	if manifest.Rows != 0 && len(records)-1 != manifest.Rows {
		return Snapshot{}, fmt.Errorf("共享盘操作日志行数不符（文件 %d，manifest %d）",
			len(records)-1, manifest.Rows)
	}
	return Snapshot{Columns: records[0], Rows: records[1:], Manifest: manifest}, nil
}

// Publish 把整份日志写到共享盘（原子写：临时文件 + rename），返回新的 manifest。
func Publish(root string, columns []string, rows [][]string, updatedBy string) (Manifest, error) {
	csvPath, manifestPath := Paths(root)
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		return Manifest{}, err
	}
	content, err := renderCSV(columns, rows)
	if err != nil {
		return Manifest{}, err
	}
	sum := sha256.Sum256(content)
	manifest := Manifest{
		Rows: len(rows), SHA256: hex.EncodeToString(sum[:]),
		UpdatedAt: time.Now().Format("2006-01-02 15:04:05"), UpdatedBy: updatedBy,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeAtomic(csvPath, content); err != nil {
		return Manifest{}, err
	}
	if err := writeAtomic(manifestPath, append(manifestBytes, '\n')); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Merge 合并两份日志：left 在前，right 里重复的行丢掉（返回合并结果与新增行数）。
func Merge(left [][]string, right [][]string) ([][]string, int) {
	seen := map[string]bool{}
	merged := make([][]string, 0, len(left)+len(right))
	add := func(row []string) bool {
		key := rowKey(row)
		if seen[key] {
			return false
		}
		seen[key] = true
		merged = append(merged, row)
		return true
	}
	for _, row := range left {
		add(row)
	}
	added := 0
	for _, row := range right {
		if add(row) {
			added++
		}
	}
	return merged, added
}

func rowKey(row []string) string {
	sum := sha256.Sum256([]byte(strings.Join(row, "\x00")))
	return hex.EncodeToString(sum[:])
}

func renderCSV(columns []string, rows [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(columns); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeAtomic(path string, content []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}
