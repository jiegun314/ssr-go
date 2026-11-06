package app

// 数据更新邮件通知：RA / EAMA / UDI 导入成功后，把本次推送的时间、数据量与基础信息
// 发给 Supply Chain 与 Admin 同事（本机 Outlook，不发 SMTP）。

import (
	"fmt"
	"strings"
	"time"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/mail"
	"github.com/jiegun314/ssr-go/internal/numfmt"
	"github.com/jiegun314/ssr-go/internal/share"
)

// NotificationPreview 是发送前给界面确认用的内容（收件人、主题、正文）。
type NotificationPreview struct {
	Source    string   `json:"source"`
	To        []string `json:"to"`
	Subject   string   `json:"subject"`
	Body      string   `json:"body"`
	Transport string   `json:"transport"`
	Mode      string   `json:"mode"`
	Ready     bool     `json:"ready"`
	Reason    string   `json:"reason,omitempty"`
}

// NotificationResult 是发送结果。
type NotificationResult struct {
	Failed  bool     `json:"failed"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	To      []string `json:"to,omitempty"`
	Mode    string   `json:"mode,omitempty"`
	Log     string   `json:"log"`
}

type notificationContent struct {
	source  string
	title   string
	to      []string
	subject string
	body    string
	ready   bool
	reason  string
}

// PreviewImportNotification 生成确认弹窗的内容（不发送）。
func (app *App) PreviewImportNotification(source string) NotificationPreview {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return NotificationPreview{Source: source, Reason: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorizeSource(source, true); err != nil {
		return NotificationPreview{Source: source, Reason: userMessage(err)}
	}
	content := app.buildNotification(source)
	return NotificationPreview{
		Source: source, To: content.to, Subject: content.subject, Body: content.body,
		Transport: app.shareConfig.EmailTransport, Mode: app.shareConfig.EmailMode,
		Ready: content.ready, Reason: content.reason,
	}
}

// SendImportNotification 通过本机 Outlook 发送数据更新通知。
func (app *App) SendImportNotification(source string) NotificationResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	result := NotificationResult{Log: app.logText()}
	if err := app.beginOperation(); err != nil {
		result.Failed, result.Title, result.Message = true, "错误", userMessage(err)
		return result
	}
	defer app.endOperation()
	if err := app.authorizeSource(source, true); err != nil {
		result.Failed, result.Title, result.Message = true, "无权限", userMessage(err)
		result.Log = app.logText()
		return result
	}
	content := app.buildNotification(source)
	result.To, result.Mode = content.to, app.shareConfig.EmailMode
	if !content.ready {
		result.Failed, result.Title, result.Message = true, "暂不能发送", content.reason
		result.Log = app.logText()
		return result
	}
	if strings.TrimSpace(app.shareConfig.EmailTransport) != "outlook" {
		result.Failed, result.Title = true, "邮件通道未启用"
		result.Message = "请在 workflow.yaml 把 email.transport 改成 outlook（或在「参数设定 → 共享盘」里改）后再试。"
		result.Log = app.logText()
		return result
	}
	if app.mailer == nil {
		result.Failed, result.Title = true, "邮件通道不可用"
		result.Message = "本机没有可用的 Outlook 发送器，请确认已安装并登录 Outlook。"
		result.Log = app.logText()
		return result
	}
	message := mail.Message{
		To: content.to, Subject: content.subject, Body: content.body, Mode: app.shareConfig.EmailMode,
	}
	if err := app.mailer.Send(message); err != nil {
		app.audit("notify", source, "邮件发送失败："+err.Error(), "failed")
		app.appendLog(LogError, "数据更新通知发送失败："+err.Error())
		result.Failed, result.Title = true, "邮件发送失败"
		result.Message = "本机 Outlook 发送失败。请确认 Outlook 已登录；如果被安全策略拦截，" +
			"可在 workflow.yaml 把 email.send_mode 改成 draft 后重试（打开草稿手动发送）。\n详情见操作日志。"
		result.Log = app.logText()
		return result
	}
	app.audit("notify", source, fmt.Sprintf("收件人=%s 模式=%s", strings.Join(content.to, "; "), message.Mode), "ok")
	modeText := "已发送"
	if message.Mode == "draft" {
		modeText = "已打开邮件草稿"
	}
	app.appendLog(LogSuccess, fmt.Sprintf("%s更新通知%s：%s", content.title, modeText, strings.Join(content.to, "; ")))
	result.Title = "邮件已发送"
	result.Message = fmt.Sprintf("%s通知已发送给 %d 位同事。", content.title, len(content.to))
	if message.Mode == "draft" {
		result.Title = "已打开邮件草稿"
		result.Message = "已在 Outlook 打开草稿（收件人/主题/正文已填好），请检查后点发送。"
	}
	result.Log = app.logText()
	return result
}

// buildNotification 汇总本次导入的推送信息（时间、数据量、发布版本、操作人、共享盘状态）。
func (app *App) buildNotification(source string) notificationContent {
	content := notificationContent{source: source}
	rule, ok := app.importer.Rules[source]
	if !ok {
		content.reason = "找不到这个数据来源。"
		return content
	}
	content.title = rule.ChineseName
	state, ok := app.importStates[source]
	if !ok || state.State == "empty" || state.RowCount == 0 {
		content.reason = "这个来源还没有导入数据，导入成功后才有可发送的更新信息。"
		return content
	}
	to := app.notificationRecipients()
	if len(to) == 0 {
		content.reason = "没有收件人：请在「参数设定 → 共享盘」填写 recipients.supply_chain / recipients.admins，" +
			"或在 users.yaml 里给 Supply Chain / Admin 填写邮箱。"
		return content
	}
	rows := state.RowCount
	cleaned := 0
	if rule.Clean != nil {
		if count, err := app.repo.CountTableRows(rule.Clean.TargetTable); err == nil {
			cleaned = count
		}
	}

	importTime := state.ImportTime
	if importTime == "" {
		importTime = "（上一次运行导入）"
	}
	versionLine := "未发布（本地模式）"
	if app.shareEnabled() {
		if shareState, err := share.LoadState(app.shareConfig.Root); err == nil {
			if version := shareState.Sources[source]; version != nil && version.Version != "" {
				versionLine = fmt.Sprintf("%s（%s 发布，%s 行）", version.Version,
					version.PublishedBy.Name, numfmt.Format(version.Rows))
			}
		}
	}
	operator := "本机用户"
	if app.session != nil {
		operator = fmt.Sprintf("%s（%s）· 机器：%s",
			auth.Label(app.session.Role), app.session.Username, app.session.Machine)
	}
	shareLine := app.shareStatus.Detail
	if shareLine == "" {
		shareLine = "未检查"
	}
	bodyLines := []string{
		"SSR 数据更新通知",
		"",
		"数据来源：" + rule.ChineseName,
		"导入时间：" + importTime,
		fmt.Sprintf("数据量：%s 行", numfmt.Format(rows)),
	}
	if cleaned > 0 && rule.Clean != nil {
		bodyLines = append(bodyLines, fmt.Sprintf("清洗后：%s 行", numfmt.Format(cleaned)))
	}
	if state.FileName != "" {
		bodyLines = append(bodyLines, "文件："+state.FileName)
	}
	bodyLines = append(bodyLines,
		"发布版本："+versionLine,
		"操作人："+operator,
		"共享盘："+shareLine,
		"",
		"发送时间："+time.Now().Format("2006/01/02 15:04"),
	)
	content.to = to
	content.subject = "SSR数据更新 - " + teamLabel(source)
	content.body = strings.Join(bodyLines, "\n")
	content.ready = true
	return content
}

// notificationRecipients 汇总 Supply Chain 与 Admin 收件人：
// 先用 workflow.yaml 的 recipients 列表；某组为空时回退到 users.yaml 里对应角色的邮箱。
func (app *App) notificationRecipients() []string {
	seen := map[string]bool{}
	result := []string{}
	add := func(address string) {
		address = strings.TrimSpace(address)
		if address == "" || seen[strings.ToLower(address)] {
			return
		}
		seen[strings.ToLower(address)] = true
		result = append(result, address)
	}
	// workflow.yaml 的分组名是 supply_chain / admins，角色名是 supply_chain / admin
	roleByGroup := map[string]string{"supply_chain": "supply_chain", "admins": "admin"}
	for _, group := range []string{"supply_chain", "admins"} {
		configured := app.shareConfig.Recipients[group]
		if len(configured) > 0 {
			for _, address := range configured {
				add(address)
			}
			continue
		}
		for _, user := range app.users {
			if string(user.Role) == roleByGroup[group] {
				add(user.Email)
			}
		}
	}
	return result
}

// teamLabel 是邮件主题里的团队名：RA / EAMA / UDI。
// EAMA 的两个来源（产品类别、医保代码）都归到 EAMA。
func teamLabel(source string) string {
	switch source {
	case "ra_input":
		return "RA"
	case "product_category", "medical_insurance_code":
		return "EAMA"
	case "global_udi_input":
		return "UDI"
	default:
		return source
	}
}
