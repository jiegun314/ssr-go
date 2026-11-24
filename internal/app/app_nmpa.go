package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jiegun314/ssr-go/internal/auth"
	"github.com/jiegun314/ssr-go/internal/nmpa"
)

// NMPARelease 是界面使用的发布包摘要（不直接暴露 nmpa 包类型给 Wails 生成器）。
type NMPARelease struct {
	Kind        string `json:"kind"`
	FileName    string `json:"fileName"`
	Title       string `json:"title"`
	PublishedAt string `json:"publishedAt"`
	Date        string `json:"date"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	Count       int    `json:"count,omitempty"`
}

// NMPAStatus 是药监参考库的界面状态。
type NMPAStatus struct {
	SocialCreditCode string        `json:"socialCreditCode"`
	CodeConfigured   bool          `json:"codeConfigured"`
	FullRelease      *NMPARelease  `json:"fullRelease,omitempty"`
	WeeklyReleases   []NMPARelease `json:"weeklyReleases,omitempty"`

	LastFullFile    string `json:"lastFullFile"`
	LastFullDate    string `json:"lastFullDate"`
	LastFullAt      string `json:"lastFullAt"`
	LastWeekEnd     string `json:"lastWeekEnd"`
	LastWeeklyFile  string `json:"lastWeeklyFile"`
	LastSyncAt      string `json:"lastSyncAt"`
	LastCheckAt     string `json:"lastCheckAt"`
	LastError       string `json:"lastError"`
	DeviceCount     int    `json:"deviceCount"`
	PackageCount    int    `json:"packageCount"`
	ContactCount    int    `json:"contactCount"`
	DatabasePath    string `json:"databasePath"`
	DatabaseBytes   int64  `json:"databaseBytes"`
	NeedsFull       bool   `json:"needsFull"`
	CodeChanged     bool   `json:"codeChanged"`
	WeeklyAvailable bool   `json:"weeklyAvailable"`
	FullDue         bool   `json:"fullDue"`
	Error           string `json:"error"`
}

// NMPASyncResult 是全量/周更同步的界面结果。
type NMPASyncResult struct {
	Mode     string     `json:"mode"`
	FileName string     `json:"fileName"`
	Matched  int        `json:"matched"`
	Devices  int        `json:"devices"`
	Packages int        `json:"packages"`
	Contacts int        `json:"contacts"`
	Message  string     `json:"message"`
	Failed   bool       `json:"failed"`
	Status   NMPAStatus `json:"status"`
}

// NMPAQueryResult 是参考库查询结果；用列/行而不是 47 字段对象，界面只展示关键列。
type NMPAQueryResult struct {
	Total   int        `json:"total"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (app *App) nmpaServiceOrError() (*nmpa.Service, error) {
	if app.nmpaService == nil {
		return nil, fmt.Errorf("药监参考库未初始化，请重启程序后重试")
	}
	return app.nmpaService, nil
}

func (app *App) nmpaContext() context.Context {
	if app.context != nil {
		return app.context
	}
	return context.Background()
}

// NMPAStatus 返回本地参考库状态（管理员）。
func (app *App) NMPAStatus() (NMPAStatus, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionNMPA); err != nil {
		return NMPAStatus{}, err
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPAStatus{}, err
	}
	status, err := service.Status()
	return toNMPAStatus(status), err
}

// NMPACheck 拉取药监 RSS，仅更新检查状态，不下载发布包。
func (app *App) NMPACheck() NMPAStatus {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionNMPA); err != nil {
		return NMPAStatus{Error: userMessage(err)}
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPAStatus{Error: userMessage(err)}
	}
	status, err := service.Check(app.nmpaContext())
	result := toNMPAStatus(status)
	if err != nil {
		result.Error = userMessage(err)
		app.appendLog(LogWarning, "药监更新检查失败："+err.Error())
		return result
	}
	app.audit("nmpa_check", "nmpa", "检查药监 UDI 发布索引", "ok")
	if status.FullRelease != nil {
		app.appendLog(LogInfo, fmt.Sprintf("药监 UDI 索引：全量 %s，周更 %d 个",
			status.FullRelease.FileName, len(status.WeeklyReleases)))
	}
	return result
}

// NMPASyncFull 下载最新全量包，按统一社会信用代码过滤后替换本地参考库。
func (app *App) NMPASyncFull() NMPASyncResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return NMPASyncResult{Mode: "full", Failed: true, Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionNMPA); err != nil {
		app.audit("deny", "nmpa", "全量同步", "denied")
		return NMPASyncResult{Mode: "full", Failed: true, Message: userMessage(err)}
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPASyncResult{Mode: "full", Failed: true, Message: userMessage(err)}
	}
	app.appendLog(LogInfo, "药监 UDI：开始下载全量包并筛选当前企业数据")
	result, err := service.SyncFull(app.nmpaContext())
	if err != nil {
		app.audit("nmpa_full", "nmpa", "全量同步失败："+err.Error(), "failed")
		app.appendLog(LogError, "药监全量同步失败："+err.Error())
		return NMPASyncResult{Mode: "full", Failed: true, Message: userMessage(err), Status: toNMPAStatus(result.Status)}
	}
	app.audit("nmpa_full", "nmpa", fmt.Sprintf("全量同步 %s，匹配 %d 条", result.FileName, result.Matched), "ok")
	app.appendLog(LogSuccess, result.Message)
	return toNMPASyncResult(result, false)
}

// NMPASyncWeekly 按顺序应用周更；发现断档时提示先做全量同步。
func (app *App) NMPASyncWeekly() NMPASyncResult {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.beginOperation(); err != nil {
		return NMPASyncResult{Mode: "weekly", Failed: true, Message: userMessage(err)}
	}
	defer app.endOperation()
	if err := app.authorize(auth.ActionNMPA); err != nil {
		app.audit("deny", "nmpa", "周更同步", "denied")
		return NMPASyncResult{Mode: "weekly", Failed: true, Message: userMessage(err)}
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPASyncResult{Mode: "weekly", Failed: true, Message: userMessage(err)}
	}
	app.appendLog(LogInfo, "药监 UDI：检查并应用周更包")
	result, err := service.SyncWeekly(app.nmpaContext())
	if err != nil {
		app.audit("nmpa_weekly", "nmpa", "周更同步失败："+err.Error(), "failed")
		app.appendLog(LogError, "药监周更同步失败："+err.Error())
		return NMPASyncResult{Mode: "weekly", Failed: true, Message: userMessage(err), Status: toNMPAStatus(result.Status)}
	}
	app.audit("nmpa_weekly", "nmpa", fmt.Sprintf("周更同步 %s，写入 %d 条", result.FileName, result.Matched), "ok")
	app.appendLog(LogSuccess, result.Message)
	return toNMPASyncResult(result, false)
}

// NMPASearch 查询本地参考库（管理员）。
func (app *App) NMPASearch(query string, page int, pageSize int) (NMPAQueryResult, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.authorize(auth.ActionNMPA); err != nil {
		return NMPAQueryResult{}, err
	}
	service, err := app.nmpaServiceOrError()
	if err != nil {
		return NMPAQueryResult{}, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	result, err := service.Query(strings.TrimSpace(query), pageSize, (page-1)*pageSize)
	if err != nil {
		return NMPAQueryResult{}, err
	}
	rows := make([][]string, 0, len(result.Devices))
	for _, device := range result.Devices {
		rows = append(rows, []string{
			device.PrimaryDI, device.ProductName, device.DeviceGenericName, device.Specification,
			device.RegistrantName, device.RegistrationFilingCertificateNo,
			device.ProductCategory, device.VersionStatus,
			fmt.Sprintf("%d", device.PackageCount), fmt.Sprintf("%d", device.ContactCount),
		})
	}
	return NMPAQueryResult{
		Total:   result.Total,
		Columns: []string{"Primary DI", "产品名称", "通用名称", "规格型号", "注册人/备案人", "注册证/备案凭证编号", "产品类别", "版本状态", "包装数", "联系人"},
		Rows:    rows,
	}, nil
}

func toNMPAStatus(status nmpa.Status) NMPAStatus {
	view := NMPAStatus{
		SocialCreditCode: status.SocialCreditCode,
		CodeConfigured:   status.CodeConfigured,
		LastFullFile:     status.LastFullFile,
		LastFullDate:     status.LastFullDate,
		LastFullAt:       status.LastFullAt,
		LastWeekEnd:      status.LastWeekEnd,
		LastWeeklyFile:   status.LastWeeklyFile,
		LastSyncAt:       status.LastSyncAt,
		LastCheckAt:      status.LastCheckAt,
		LastError:        status.LastError,
		DeviceCount:      status.DeviceCount,
		PackageCount:     status.PackageCount,
		ContactCount:     status.ContactCount,
		DatabasePath:     status.DatabasePath,
		DatabaseBytes:    status.DatabaseBytes,
		NeedsFull:        status.NeedsFull,
		CodeChanged:      status.CodeChanged,
		WeeklyAvailable:  status.WeeklyAvailable,
		FullDue:          status.FullDue,
	}
	if status.FullRelease != nil {
		view.FullRelease = toNMPARelease(*status.FullRelease)
	}
	for _, release := range status.WeeklyReleases {
		view.WeeklyReleases = append(view.WeeklyReleases, *toNMPARelease(release))
	}
	return view
}

func toNMPARelease(release nmpa.Release) *NMPARelease {
	return &NMPARelease{
		Kind: string(release.Kind), FileName: release.FileName, Title: release.Title,
		PublishedAt: release.PublishedAt, Date: release.Date, Start: release.Start,
		End: release.End, Count: release.Count,
	}
}

func toNMPASyncResult(result nmpa.SyncResult, failed bool) NMPASyncResult {
	return NMPASyncResult{
		Mode: result.Mode, FileName: result.FileName, Matched: result.Matched,
		Devices: result.Devices, Packages: result.Packages, Contacts: result.Contacts,
		Message: result.Message, Failed: failed, Status: toNMPAStatus(result.Status),
	}
}
