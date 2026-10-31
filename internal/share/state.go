package share

// 共享盘上的协作状态 state/state.json（设计文档 §4.3/§4.4）：
//
//   - 每个来源一条"当前版本"（谁、何时、多少行、sha256）；
//   - 最后一次整合用了哪四个版本、结果文件与计数；
//   - 正在整合时的锁记录（P4 用）；
//   - generation 每次状态变更 +1（用来发现"整合期间有人发布"）。
//
// 写入必须走 WithStateLock + 原子写（临时文件 + rename），否则并发发布会丢更新。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Owner 是一次操作的人（登录时选的组别 + 本机用户名 + 机器）。
type Owner struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Username string `json:"username"`
	Machine  string `json:"machine"`
}

// SourceVersion 是某个来源当前发布的版本。
type SourceVersion struct {
	Version     string   `json:"version"`
	SHA256      string   `json:"sha256"`
	Rows        int      `json:"rows"`
	CleanedRows int      `json:"cleaned_rows,omitempty"`
	Columns     []string `json:"columns,omitempty"`
	PublishedAt string   `json:"published_at"`
	PublishedBy Owner    `json:"published_by"`
}

// Counts 是一次整合的状态计数。
type Counts struct {
	Ready     int `json:"ready"`
	Missing   int `json:"missing"`
	Duplicate int `json:"duplicate"`
	Conflict  int `json:"conflict"`
}

// EmailStatus 是整合结果的发送状态（邮件 P7 才实现，当前固定 disabled）。
type EmailStatus struct {
	Status     string   `json:"status"`
	Recipients []string `json:"recipients,omitempty"`
	At         string   `json:"at,omitempty"`
}

// Consolidation 是最后一次整合的记录。
type Consolidation struct {
	ID       string            `json:"id"`
	At       string            `json:"at"`
	By       Owner             `json:"by"`
	Versions map[string]string `json:"versions"`
	Result   string            `json:"result"`
	Counts   Counts            `json:"counts"`
	Email    EmailStatus       `json:"email"`
}

// Running 是"正在整合"的锁记录（P4 使用）。
type Running struct {
	ID        string `json:"id"`
	Owner     Owner  `json:"owner"`
	Since     string `json:"since"`
	Heartbeat string `json:"heartbeat"`
}

// State 是 state.json 的内容。
type State struct {
	Generation           int                       `json:"generation"`
	Sources              map[string]*SourceVersion `json:"sources"`
	LastConsolidation    *Consolidation            `json:"last_consolidation"`
	ConsolidationRunning *Running                  `json:"consolidation_running"`
}

// StatePath 返回 state.json 的路径。
func StatePath(root string) string {
	return filepath.Join(root, "state", "state.json")
}

// LoadState 读 state.json；文件不存在时返回空状态（第一次使用）。
func LoadState(root string) (*State, error) {
	content, err := os.ReadFile(StatePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Sources: map[string]*SourceVersion{}}, nil
		}
		return nil, fmt.Errorf("读取共享盘状态失败：%w", err)
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, fmt.Errorf("共享盘状态文件损坏（state/state.json）：%w", err)
	}
	if state.Sources == nil {
		state.Sources = map[string]*SourceVersion{}
	}
	return &state, nil
}

// Save 原子写 state.json（临时文件 + rename）。
func (state *State) Save(root string) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化共享盘状态失败：%w", err)
	}
	content = append(content, '\n')
	return writeFileAtomic(StatePath(root), content)
}

// WithStateLock 在 state/state.lock 的独占锁里执行 fn（读改写状态）。
//
// 锁用 O_CREAT|O_EXCL 创建；超过 10 秒的锁视为残留、可回收；拿不到锁重试 2 秒后报错。
func WithStateLock(root string, fn func() error) error {
	lockPath := filepath.Join(root, "state", "state.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("共享盘 state 目录不可用：%w", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		handle, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(handle, "pid=%d at=%d", os.Getpid(), time.Now().UnixNano())
			handle.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return fmt.Errorf("共享盘 state.lock 创建失败：%w", err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 10*time.Second {
			os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("共享盘状态文件被占用（state.lock），请稍后重试")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// writeFileAtomic 先写临时文件再 rename（网络盘上避免读到半个文件）。
func writeFileAtomic(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("创建目录失败 %s：%w", directory, err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, content, 0o644); err != nil {
		return fmt.Errorf("写入临时文件失败 %s：%w", temp, err)
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return fmt.Errorf("原子写入失败 %s：%w", path, err)
	}
	return nil
}

// Department 把来源键映射成共享盘上的部门目录（published/<部门>/）。
func Department(source string) string {
	switch source {
	case "ra_input":
		return "ra"
	case "product_category", "medical_insurance_code":
		return "eama"
	case "global_udi_input":
		return "udi"
	}
	return source
}
