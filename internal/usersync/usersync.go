// Package usersync 把 Admin 维护的人员名单同步到共享盘，供其它电脑拉取。
//
// 共享盘上的位置：
//
//	<share>/config/users.yaml
//	<share>/config/users.manifest.json   （sha256 + 版本 + 操作人）
//
// 只有 Admin 的保存会发布；其它电脑启动/登录时按 manifest 校验并拉取。
package usersync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// 文件名与目录都是对外约定（共享盘上的结构）。
const (
	Directory    = "config"
	FileName     = "users.yaml"
	ManifestName = "users.manifest.json"
)

// ErrNotFound 表示共享盘上还没有人员名单（第一次发布之前）。
var ErrNotFound = errors.New("共享盘上还没有人员名单")

// Manifest 是共享盘人员名单的版本信息。
type Manifest struct {
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

// Hash 是人员名单内容的 sha256（十六进制小写）。
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func paths(root string) (string, string, string) {
	directory := filepath.Join(root, Directory)
	return directory, filepath.Join(directory, FileName), filepath.Join(directory, ManifestName)
}

// Publish 把人员名单写到共享盘：先内容、后 manifest（manifest 永远指向已写好的内容）。
// 内容与当前版本一致时不重复写，直接返回现有 manifest。
func Publish(root string, content []byte, updatedBy string) (Manifest, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Manifest{}, fmt.Errorf("共享盘根目录不可用：%s", root)
	}
	directory, target, manifestPath := paths(root)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("创建共享盘配置目录失败：%v", err)
	}
	hash := Hash(content)
	if _, current, err := Fetch(root); err == nil && current.SHA256 == hash {
		return current, nil
	}
	if err := writeAtomic(target, content); err != nil {
		return Manifest{}, fmt.Errorf("写人员名单失败：%v", err)
	}
	manifest := Manifest{
		Version:   time.Now().UTC().Format("20060102T150405Z") + "-" + hash[:8],
		SHA256:    hash,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedBy: updatedBy,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeAtomic(manifestPath, append(encoded, '\n')); err != nil {
		return Manifest{}, fmt.Errorf("写人员名单 manifest 失败：%v", err)
	}
	return manifest, nil
}

// Fetch 读取并校验共享盘上的人员名单。
func Fetch(root string) ([]byte, Manifest, error) {
	_, contentPath, manifestPath := paths(root)
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Manifest{}, ErrNotFound
		}
		return nil, Manifest{}, fmt.Errorf("读人员名单 manifest 失败：%v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return nil, Manifest{}, fmt.Errorf("人员名单 manifest 格式不对：%v", err)
	}
	content, err := os.ReadFile(contentPath)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("读人员名单失败：%v", err)
	}
	if Hash(content) != manifest.SHA256 {
		return nil, Manifest{}, fmt.Errorf("人员名单校验失败（内容与 manifest 不一致）")
	}
	return content, manifest, nil
}

// writeAtomic 用同目录临时文件 + rename，避免另一端读到写了一半的文件。
func writeAtomic(path string, content []byte) error {
	handle, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temp := handle.Name()
	defer os.Remove(temp)
	if _, err := handle.Write(content); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Sync(); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
