package share

// 共享盘上的整合租约：防止两个 Supply Chain 同时整合。
//
// 两处记录配合使用：
//   - state/consolidate.lock 文件：O_CREAT|O_EXCL，进程崩溃后留下的文件由 TTL 回收；
//   - state.json 的 consolidation_running：记录"谁在整合、从什么时候开始"，界面据此提示。

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ConsolidationLockTTL 是崩溃后残留租约的回收时间。
const ConsolidationLockTTL = 30 * time.Minute

func consolidationLockPath(root string) string {
	return filepath.Join(root, "state", "consolidate.lock")
}

// AcquireConsolidation 抢占整合租约；有人在整合时返回错误（含对方是谁）。
func AcquireConsolidation(root string, id string, owner Owner) error {
	lockPath := consolidationLockPath(root)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("共享盘 state 目录不可用：%w", err)
	}
	// 被占用时：看 state 里的心跳是否过期；过期就回收
	if err := tryCreateLock(lockPath, id); err != nil {
		state, loadErr := LoadState(root)
		if loadErr == nil && state.ConsolidationRunning != nil {
			running := state.ConsolidationRunning
			started, parseErr := time.ParseInLocation("2006/01/02 15:04:05", running.Heartbeat, time.Local)
			if parseErr == nil && time.Since(started) < ConsolidationLockTTL {
				return fmt.Errorf("「%s」正在整合（开始于 %s），请稍后再试",
					ownerLabel(running.Owner), running.Since)
			}
		}
		// 残留（解析失败或已过期）：回收后重试一次
		os.Remove(lockPath)
		if err := tryCreateLock(lockPath, id); err != nil {
			return fmt.Errorf("共享盘整合锁被占用，请稍后重试")
		}
	}
	now := time.Now().Format("2006/01/02 15:04:05")
	err := WithStateLock(root, func() error {
		state, err := LoadState(root)
		if err != nil {
			return err
		}
		state.ConsolidationRunning = &Running{
			ID: id, Owner: owner, Since: now, Heartbeat: now,
		}
		state.Generation++
		return state.Save(root)
	})
	if err != nil {
		os.Remove(lockPath)
		return err
	}
	return nil
}

// ReleaseConsolidation 释放租约（只有在持有者 id 匹配时清 state）。
func ReleaseConsolidation(root string, id string) {
	os.Remove(consolidationLockPath(root))
	_ = WithStateLock(root, func() error {
		state, err := LoadState(root)
		if err != nil {
			return err
		}
		if state.ConsolidationRunning != nil && state.ConsolidationRunning.ID == id {
			state.ConsolidationRunning = nil
			state.Generation++
			return state.Save(root)
		}
		return nil
	})
}

func tryCreateLock(path string, id string) error {
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	fmt.Fprintf(handle, "id=%s pid=%d at=%d", id, os.Getpid(), time.Now().UnixNano())
	handle.Close()
	return nil
}

func ownerLabel(owner Owner) string {
	if owner.Name == "" {
		return "未知用户"
	}
	if owner.Role == "" {
		return owner.Name
	}
	return owner.Name + "（" + owner.Role + "）"
}
