// Package share 负责"共享盘是否可用"的检查（P1 只做骨架）。
//
// 口径（docs/design/multiuser-workflow.md §6）：
//   - workflow.yaml 的 share.root 未配置（空串）→ **本地模式**，始终健康；
//   - 配置了 → 检查目录存在、可写、支持"临时文件 + rename"（发布与状态写入都依赖它）；
//   - 检查带超时，避免网络盘卡死把界面拖住。
//
// 完整的目录结构/锁/时钟检查在 P2 补齐；本包先把"健康 / 不健康 / 原因"稳定下来。
package share

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/config"
)

// Config 是 workflow.yaml 里与共享盘相关的配置。
type Config struct {
	Root                     string
	PollSeconds              int
	ProbeTimeout             time.Duration
	SyncAuto                 bool
	RequireBeforeConsolidate bool
}

// Load 读 workflow.yaml；没有这个文件时返回零值（= 本地模式）。
func Load(loader *config.Loader) (Config, error) {
	document, err := loader.LoadWorkflow()
	if err != nil || document == nil {
		return Config{}, err
	}
	block, _ := document["share"].(map[string]any)
	sync, _ := document["sync"].(map[string]any)
	configValue := Config{
		Root:                     strings.TrimSpace(text(block["root"])),
		PollSeconds:              integer(block["poll_seconds"], 30),
		ProbeTimeout:             time.Duration(integer(block["probe_timeout_seconds"], 5)) * time.Second,
		SyncAuto:                 boolean(sync["auto"], true),
		RequireBeforeConsolidate: boolean(sync["require_before_consolidate"], true),
	}
	return configValue, nil
}

// Enabled 表示配置了共享盘（否则是本地模式）。
func (config Config) Enabled() bool { return config.Root != "" }

// Status 是一次检查的结果（界面用来决定是否开放操作权限）。
type Status struct {
	Mode      string `json:"mode"` // local | share
	Healthy   bool   `json:"healthy"`
	Detail    string `json:"detail"`
	CheckedAt string `json:"checkedAt"`
}

// Check 检查共享盘。本地模式直接返回健康；共享盘模式带超时探测。
func (config Config) Check() Status {
	checkedAt := time.Now().Format("2006/01/02 15:04:05")
	if !config.Enabled() {
		return Status{Mode: "local", Healthy: true, Detail: "本地模式：未配置共享盘", CheckedAt: checkedAt}
	}
	timeout := config.ProbeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	result := make(chan string, 1)
	go func() { result <- probe(config.Root) }()
	select {
	case detail := <-result:
		if detail == "" {
			return Status{Mode: "share", Healthy: true, Detail: "共享盘正常：" + config.Root, CheckedAt: checkedAt}
		}
		return Status{Mode: "share", Healthy: false, Detail: detail, CheckedAt: checkedAt}
	case <-time.After(timeout):
		return Status{
			Mode: "share", Healthy: false, CheckedAt: checkedAt,
			Detail: fmt.Sprintf("共享盘检查超时（%d 秒）：%s", int(timeout.Seconds()), config.Root),
		}
	}
}

// probe 返回空串表示正常，否则是给用户看的中文原因。
func probe(root string) string {
	info, err := os.Stat(root)
	if err != nil {
		return "共享盘不可达：" + err.Error()
	}
	if !info.IsDir() {
		return "共享盘路径不是目录：" + root
	}
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		return "共享盘的 state 目录不可用：" + err.Error()
	}
	name := fmt.Sprintf(".probe-%d-%d", os.Getpid(), time.Now().UnixNano())
	temp := filepath.Join(state, name+".tmp")
	done := filepath.Join(state, name+".ok")
	if err := os.WriteFile(temp, []byte("ok"), 0o644); err != nil {
		return "共享盘不可写：" + err.Error()
	}
	if err := os.Rename(temp, done); err != nil {
		os.Remove(temp)
		return "共享盘不支持原子写（临时文件 rename 失败）：" + err.Error()
	}
	if err := os.Remove(done); err != nil {
		return "共享盘不可删除探针文件：" + err.Error()
	}
	return ""
}

func text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func integer(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolean(value any, fallback bool) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fallback
}
