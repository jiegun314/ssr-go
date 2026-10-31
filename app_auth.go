package main

// 多人协作版的身份、权限与审计（P1）。
//
// 口径见 docs/design/multiuser-workflow.md §5/§12：
//   - 登录时选择身份（不读域信息，只记录本机用户名）；
//   - 除 Admin 外无密码；Admin 密码写死在 internal/auth（哈希常量）；
//   - 没有 users.yaml = 单机模式：自动以 Admin 身份进入，行为与旧版本一致；
//   - 每个绑定方法用 authorize 统一裁决，拒绝也要写审计。

import (
	"fmt"
	"strings"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/share"
	"github.com/jiegun314/ssr-go/internal/store"
)

// LoginUser 是登录页展示的一行（不含邮箱等敏感字段）。
type LoginUser struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	RoleLabel     string `json:"roleLabel"`
	NeedsPassword bool   `json:"needsPassword"`
	Note          string `json:"note"`
}

// SessionInfo 是登录后的身份信息（前端按 actions 决定按钮显隐）。
type SessionInfo struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	RoleLabel   string   `json:"roleLabel"`
	Username    string   `json:"username"`
	Machine     string   `json:"machine"`
	At          string   `json:"at"`
	SingleUser  bool     `json:"singleUser"`
	ShareReady  bool     `json:"shareReady"`
	ShareDetail string   `json:"shareDetail"`
	Actions     []string `json:"actions"`
}

// LoginResult 是登录/切换身份的结果。
type LoginResult struct {
	Failed  bool         `json:"failed"`
	Title   string       `json:"title"`
	Message string       `json:"message"`
	Log     string       `json:"log"`
	Session *SessionInfo `json:"session"`
}

// ListUsers 返回登录页可选的人（单机模式返回空，前端直接进主界面）。
func (app *App) ListUsers() []LoginUser {
	app.mu.Lock()
	defer app.mu.Unlock()
	return loginUsers(app.users)
}

// loginUsers 把人员列表转成登录页结构（InitialState 与 ListUsers 共用）。
func loginUsers(users []auth.User) []LoginUser {
	converted := make([]LoginUser, 0, len(users))
	for _, user := range users {
		converted = append(converted, LoginUser{
			Name:          user.Name,
			Role:          string(user.Role),
			RoleLabel:     auth.Label(user.Role),
			NeedsPassword: user.Role == auth.RoleAdmin,
			Note:          user.Note,
		})
	}
	return converted
}

// Login 选择身份进入。Admin 需要密码；其余角色选择即可。
func (app *App) Login(name string, password string) LoginResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return LoginResult{Failed: true, Title: "错误", Message: userMessage(err)}
	}
	defer app.endOperation()
	if len(app.users) == 0 {
		// 单机模式：没有名单可登，直接返回当前（自动建立的）身份
		return LoginResult{Log: app.logText(), Session: app.sessionInfo()}
	}
	var selected *auth.User
	for index := range app.users {
		if app.users[index].Name == name {
			selected = &app.users[index]
			break
		}
	}
	if selected == nil {
		app.audit("login", name, "找不到这个用户", "denied")
		return LoginResult{Failed: true, Title: "登录失败", Message: "找不到这个用户，请重新选择。"}
	}
	if selected.Role == auth.RoleAdmin && !auth.VerifyAdminPassword(password) {
		reason := "Admin 密码不正确"
		if strings.TrimSpace(password) == "" {
			reason = "Admin 登录需要密码"
		}
		app.audit("login", selected.Name, reason, "denied")
		return LoginResult{Failed: true, Title: "登录失败", Message: reason + "。"}
	}
	app.session = &auth.Session{
		Name: selected.Name, Role: selected.Role,
		Username: auth.CurrentUsername(), Machine: auth.CurrentMachine(), At: auth.Now(),
	}
	status := app.checkShare()
	app.audit("login", selected.Name, fmt.Sprintf("角色=%s 共享盘=%s", auth.Label(selected.Role), status.Detail), "ok")
	app.appendLog(LogInfo, fmt.Sprintf("已登录：%s（%s）", selected.Name, auth.Label(selected.Role)))
	return LoginResult{Log: app.logText(), Session: app.sessionInfo()}
}

// Logout 退出当前身份（单机模式下没有意义，直接返回当前身份）。
func (app *App) Logout() LoginResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.users) == 0 {
		return LoginResult{Log: app.logText(), Session: app.sessionInfo()}
	}
	if app.session != nil {
		app.audit("logout", app.session.Name, "", "ok")
		app.appendLog(LogInfo, fmt.Sprintf("已退出：%s", app.session.Name))
		app.session = nil
	}
	return LoginResult{Log: app.logText()}
}

// sessionInfo 把当前会话转成前端结构（actions 决定按钮显隐）。
func (app *App) sessionInfo() *SessionInfo {
	app.ensureSession()
	if app.session == nil {
		return nil
	}
	return &SessionInfo{
		Name:        app.session.Name,
		Role:        string(app.session.Role),
		RoleLabel:   auth.Label(app.session.Role),
		Username:    app.session.Username,
		Machine:     app.session.Machine,
		At:          app.session.At,
		SingleUser:  app.singleUser,
		ShareReady:  app.session.ShareReady,
		ShareDetail: app.session.ShareDetail,
		Actions:     auth.Actions(app.session.Role),
	}
}

// authorize 统一裁决：没登录、或角色不允许，都返回中文原因并写审计。
func (app *App) authorize(action auth.Action) error {
	app.ensureSession()
	if app.session == nil {
		return fmt.Errorf("请先选择登录身份")
	}
	if !auth.Can(app.session.Role, action) {
		app.audit("deny", string(action), "权限不足", "denied")
		return fmt.Errorf("当前身份（%s）没有这个操作的权限", auth.Label(app.session.Role))
	}
	return nil
}

// authorizeSource 裁决"导入/查看某个来源"。wantImport 区分发布与查看。
func (app *App) authorizeSource(source string, wantImport bool) error {
	app.ensureSession()
	if app.session == nil {
		return fmt.Errorf("请先选择登录身份")
	}
	allowed := false
	if wantImport {
		allowed = auth.CanImport(app.session.Role, source)
	} else {
		allowed = auth.CanReview(app.session.Role, source)
	}
	if !allowed {
		app.audit("deny", source, "查看/导入该来源", "denied")
		return fmt.Errorf("当前身份（%s）没有这个来源的权限", auth.Label(app.session.Role))
	}
	return nil
}

// audit 写一条审计；写失败只留运行日志，不影响业务。
func (app *App) audit(action string, target string, detail string, result string) {
	if app.repo == nil {
		return
	}
	record := store.AuditRecord{Action: action, Target: target, Detail: detail, Result: result}
	record.At = auth.Now()
	if app.session != nil {
		record.Actor = app.session.Name
		record.Role = string(app.session.Role)
		record.Username = app.session.Username
		record.Machine = app.session.Machine
	} else {
		record.Actor = "未登录"
		record.Username = auth.CurrentUsername()
		record.Machine = auth.CurrentMachine()
	}
	if err := app.repo.AppendAudit(record); err != nil {
		app.appendLog(LogWarning, "审计写入失败："+err.Error())
	}
}

// checkShare 检查共享盘并把结果记到会话上（P1 只做骨架，不阻断本地操作）。
func (app *App) checkShare() share.Status {
	status := app.shareConfig.Check()
	app.shareStatus = status
	if app.session != nil {
		app.session.ShareReady = status.Healthy
		app.session.ShareDetail = status.Detail
	}
	result := "ok"
	if !status.Healthy {
		result = "failed"
	}
	app.audit("share-check", status.Mode, status.Detail, result)
	if !status.Healthy {
		app.appendLog(LogWarning, "共享盘检查失败："+status.Detail)
	}
	return status
}

// ensureSession 保证"没有 users.yaml 时永远有一个 Admin 身份"：
// 单机模式的旧行为不能因为引入登录而失效（测试与未走 startup 的调用路径也走这里）。
func (app *App) ensureSession() {
	if app.session != nil || len(app.users) > 0 {
		return
	}
	app.singleUser = true
	app.session = &auth.Session{
		Name: "本机用户", Role: auth.RoleAdmin,
		Username: auth.CurrentUsername(), Machine: auth.CurrentMachine(), At: auth.Now(),
	}
	if app.shareStatus.Detail == "" {
		app.shareStatus = app.shareConfig.Check()
	}
	app.session.ShareReady = app.shareStatus.Healthy
	app.session.ShareDetail = app.shareStatus.Detail
}

// startSingleUserSession 在单机模式下建立自动身份（users.yaml 缺失时）并做一次共享盘检查。
func (app *App) startSingleUserSession() {
	app.ensureSession()
	status := app.checkShare()
	app.audit("login", "单机模式", "自动以 Admin 进入；共享盘="+status.Detail, "ok")
}
