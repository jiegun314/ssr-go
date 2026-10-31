// Package auth 是多人协作版的身份与权限：谁（角色）能做什么。
//
// 设计口径（docs/design/multiuser-workflow.md §5/§12）：
//   - 身份是**声明式**的：登录时选择，不读域信息；只记录本机用户名，用于审计；
//   - 除 Admin 外无密码；Admin 密码写死在程序里（哈希常量），换密码必须发新版本；
//   - 权限矩阵在这里集中定义，App 的每个绑定方法用 Can/CanImport 统一裁决；
//   - 没有 users.yaml 时由调用方进入"单机模式"（全权限），保证旧行为不变。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Role 是登录身份的角色。
type Role string

const (
	RoleRA          Role = "ra"
	RoleEAMA        Role = "eama"
	RoleUDI         Role = "udi"
	RoleSupplyChain Role = "supply_chain"
	RoleAdmin       Role = "admin"
)

// RoleLabels 是角色在界面上的显示名。
var RoleLabels = map[Role]string{
	RoleRA:          "RA",
	RoleEAMA:        "EAMA",
	RoleUDI:         "Global UDI Team",
	RoleSupplyChain: "Supply Chain",
	RoleAdmin:       "Admin",
}

// RoleOrder 是登录页与权限清单的分组顺序（与业务流一致）。
var RoleOrder = []Role{RoleRA, RoleEAMA, RoleUDI, RoleSupplyChain, RoleAdmin}

// Roles 是 users.yaml 允许的角色（配置校验与解析共用）。
var Roles = []string{string(RoleRA), string(RoleEAMA), string(RoleUDI), string(RoleSupplyChain), string(RoleAdmin)}

// User 是 users.yaml 里的一条人员记录。
type User struct {
	Name  string
	Role  Role
	Email string
	Note  string
}

// Action 是被权限矩阵约束的动作。
type Action string

const (
	// 导入/发布：三个部门各管自己的来源。
	ActionImportRA   Action = "import:ra_input"
	ActionImportEAMA Action = "import:eama"
	ActionImportUDI  Action = "import:global_udi_input"
	// 对齐（从共享盘拉取并校验最新版本）：Supply Chain / Admin。
	ActionAlign Action = "align"
	// 整合与导出：Supply Chain / Admin。
	ActionConsolidate Action = "consolidate"
	ActionExport      Action = "export"
	// 清空导入数据：只给 Admin。
	ActionClearData Action = "clear"
	// 参数设定：SC 只读、Admin 可写。
	ActionViewSettings Action = "settings:read"
	ActionEditSettings Action = "settings:write"
	// 共享盘地址、人员名单等：只给 Admin。
	ActionManageShare Action = "share:write"
	// 看别人的来源明细（只有 SC / Admin）。
	ActionReviewAny Action = "review:any"
	// 看全部审计（SC / Admin）。
	ActionViewAuditAny Action = "audit:any"
)

// matrix 是"角色 → 允许的动作"的唯一口径（设计文档 §12）。
var matrix = map[Role][]Action{
	RoleRA:   {ActionImportRA},
	RoleEAMA: {ActionImportEAMA},
	RoleUDI:  {ActionImportUDI},
	RoleSupplyChain: {
		ActionAlign, ActionConsolidate, ActionExport, ActionViewSettings, ActionReviewAny, ActionViewAuditAny,
	},
	RoleAdmin: {
		ActionImportRA, ActionImportEAMA, ActionImportUDI,
		ActionAlign, ActionConsolidate, ActionExport, ActionClearData,
		ActionViewSettings, ActionEditSettings, ActionManageShare,
		ActionReviewAny, ActionViewAuditAny,
	},
}

// Can 判断角色是否允许该动作。
func Can(role Role, action Action) bool {
	for _, allowed := range matrix[role] {
		if allowed == action {
			return true
		}
	}
	return false
}

// Actions 返回该角色允许的动作（按固定顺序，给前端做按钮显隐）。
func Actions(role Role) []string {
	actions := make([]string, 0, len(matrix[role]))
	for _, action := range matrix[role] {
		actions = append(actions, string(action))
	}
	sort.Strings(actions)
	return actions
}

// Label 返回角色的显示名（未知角色原样返回）。
func Label(role Role) string {
	if label, ok := RoleLabels[role]; ok {
		return label
	}
	return string(role)
}

// SourceAction 把来源键映射成"导入/发布"动作。
func SourceAction(source string) (Action, bool) {
	switch source {
	case "ra_input":
		return ActionImportRA, true
	case "product_category", "medical_insurance_code":
		return ActionImportEAMA, true
	case "global_udi_input":
		return ActionImportUDI, true
	}
	return "", false
}

// CanImport 判断角色能否导入/发布该来源（Admin 不受限）。
func CanImport(role Role, source string) bool {
	action, known := SourceAction(source)
	if !known {
		return false
	}
	return Can(role, action)
}

// CanReview 判断角色能否查看该来源的明细（管理员与 SC 可看全部）。
func CanReview(role Role, source string) bool {
	if Can(role, ActionReviewAny) {
		return true
	}
	return CanImport(role, source)
}

// IsKnownRole 判断角色写法是否合法。
func IsKnownRole(value string) bool {
	for _, role := range Roles {
		if role == value {
			return true
		}
	}
	return false
}

// ParseUsers 把 users.yaml 解析成人员列表（校验名字非空且不重复、角色合法）。
func ParseUsers(document map[string]any) ([]User, error) {
	if document == nil {
		return nil, nil
	}
	raw, _ := document["users"].([]any)
	if len(raw) == 0 {
		return nil, errors.New("users.yaml 里没有 users 列表（至少要有一个人）")
	}
	users := make([]User, 0, len(raw))
	seen := map[string]bool{}
	for position, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("users[%d] 必须是一个映射", position)
		}
		name := strings.TrimSpace(text(entry["name"]))
		if name == "" {
			return nil, fmt.Errorf("users[%d].name 不能为空", position)
		}
		if seen[name] {
			return nil, fmt.Errorf("users[%d].name 重复：%s", position, name)
		}
		seen[name] = true
		role := strings.TrimSpace(text(entry["role"]))
		if !IsKnownRole(role) {
			return nil, fmt.Errorf("users[%d].role 不合法：%s（可选 %s）",
				position, role, strings.Join(Roles, " / "))
		}
		users = append(users, User{
			Name:  name,
			Role:  Role(role),
			Email: strings.TrimSpace(text(entry["email"])),
			Note:  strings.TrimSpace(text(entry["note"])),
		})
	}
	return users, nil
}

// ---------- Admin 密码 ----------

// adminPasswordRecord 是写死在程序里的 Admin 密码记录：
//
//	pbkdf2-sha256$<迭代次数>$<盐 base64>$<哈希 base64>
//
// 只存哈希、不存明文。当前密码见 docs/design/multiuser-workflow.md §5.1 的运营说明；
// 要换密码只能改这个常量并发新版本（设计约定：密码固定不变）。
const adminPasswordRecord = "pbkdf2-sha256$120000$XqQqeMv4yrhyYTH5c36A/Q==$GCkSHU0AX6XOPGeaPDAsMXVq0Qz3GAJXJAZRHVMQTA0="

// VerifyAdminPassword 校验 Admin 密码（常量时间比较）。
func VerifyAdminPassword(password string) bool {
	if password == "" {
		return false
	}
	parts := strings.Split(adminPasswordRecord, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// pbkdf2SHA256 是 PBKDF2-HMAC-SHA256（RFC 2898），只用标准库，避免为一行校验引入依赖。
func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	const hashLength = sha256.Size
	blocks := (keyLength + hashLength - 1) / hashLength
	derived := make([]byte, 0, blocks*hashLength)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		counter := []byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)}
		mac.Write(counter)
		previous := mac.Sum(nil)
		current := make([]byte, len(previous))
		copy(current, previous)
		for round := 1; round < iterations; round++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(previous)
			previous = mac.Sum(nil)
			for index := range current {
				current[index] ^= previous[index]
			}
		}
		derived = append(derived, current...)
	}
	return derived[:keyLength]
}

// ---------- 本机身份（只记录，不校验） ----------

// CurrentUsername 返回当前电脑的操作系统用户名（不读域信息）。
func CurrentUsername() string {
	for _, name := range []string{"USERNAME", "USER", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return "unknown"
}

// CurrentMachine 返回机器名（审计里用来区分不同电脑）。
func CurrentMachine() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}

// Now 返回审计/会话用的时间串。
func Now() string { return time.Now().Format("2006/01/02 15:04:05") }

// Session 是一次登录的结果：身份 + 本机信息 + 共享盘状态。
type Session struct {
	Name        string
	Role        Role
	Username    string
	Machine     string
	At          string
	ShareReady  bool
	ShareDetail string
}

// text 读一个 YAML 标量（数字也转成字符串，兼容手写的不加引号的值）。
func text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}
