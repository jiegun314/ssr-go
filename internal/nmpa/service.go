package nmpa

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store 是参考库的最小持久化接口；SQL 实现在 internal/store。
type Store interface {
	Metadata() (map[string]string, error)
	SetMetadata(map[string]string) error
	ReplaceAll([]Device, map[string]string) error
	Upsert([]Device, map[string]string) error
	Counts() (int, int, int, error)
	Search(query string, limit, offset int) (QueryResult, error)
	Path() string
	Size() int64
}

// Progress 是下载/解析进度（界面目前只记录日志，保留给后续进度条）。
type Progress struct {
	Stage   string
	Loaded  int64
	Total   int64
	Devices int
}

// Service 负责读 RSS、下载发布包、过滤并写入参考库。
type Service struct {
	Store      Store
	Code       func() (string, error)
	Client     *http.Client
	FeedURL    string
	Now        func() time.Time
	OnProgress func(Progress)
}

// NewService 创建服务；code 每次同步时重新读取，参数设定改代码后无需重启。
func NewService(store Store, code func() (string, error)) *Service {
	return &Service{
		Store:   store,
		Code:    code,
		Client:  &http.Client{Timeout: 0},
		FeedURL: FeedURL,
		Now:     time.Now,
	}
}

const (
	metaCode           = "social_credit_code"
	metaFullFile       = "full_file"
	metaFullDate       = "full_date"
	metaFullAt         = "full_synced_at"
	metaLastWeekEnd    = "last_week_end"
	metaLastWeeklyFile = "last_weekly_file"
	metaLastSyncAt     = "last_sync_at"
	metaLastCheckAt    = "last_check_at"
	metaLastError      = "last_error"
)

// Status 读本地状态；不访问网络。
func (service *Service) Status() (Status, error) {
	if service == nil || service.Store == nil {
		return Status{}, fmt.Errorf("药监参考库未初始化")
	}
	meta, err := service.Store.Metadata()
	if err != nil {
		return Status{}, err
	}
	devices, packages, contacts, err := service.Store.Counts()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		SocialCreditCode: meta[metaCode],
		CodeConfigured:   strings.TrimSpace(meta[metaCode]) != "",
		LastFullFile:     meta[metaFullFile],
		LastFullDate:     meta[metaFullDate],
		LastFullAt:       meta[metaFullAt],
		LastWeekEnd:      meta[metaLastWeekEnd],
		LastWeeklyFile:   meta[metaLastWeeklyFile],
		LastSyncAt:       meta[metaLastSyncAt],
		LastCheckAt:      meta[metaLastCheckAt],
		LastError:        meta[metaLastError],
		DeviceCount:      devices,
		PackageCount:     packages,
		ContactCount:     contacts,
		DatabasePath:     service.Store.Path(),
		DatabaseBytes:    service.Store.Size(),
	}
	code, codeErr := service.code()
	if codeErr == nil {
		status.CodeConfigured = strings.TrimSpace(code) != ""
		if status.SocialCreditCode == "" {
			status.SocialCreditCode = strings.TrimSpace(code)
		}
		if status.CodeConfigured && !strings.EqualFold(strings.TrimSpace(code), strings.TrimSpace(status.SocialCreditCode)) {
			status.CodeChanged = true
		}
	}
	status.NeedsFull = status.LastFullDate == "" || status.CodeChanged
	return status, nil
}

// Check 拉取 RSS 并把可用发布包附在状态上；本地数据不变。
func (service *Service) Check(ctx context.Context) (Status, error) {
	status, err := service.Status()
	if err != nil {
		return Status{}, err
	}
	code, err := service.code()
	if err != nil {
		return status, err
	}
	status.SocialCreditCode = strings.TrimSpace(code)
	status.CodeConfigured = status.SocialCreditCode != ""
	if !status.CodeConfigured {
		return status, fmt.Errorf("请先在参数设定中填写统一社会信用代码")
	}
	feed, err := service.fetchFeed(ctx)
	if err != nil {
		service.saveMeta(map[string]string{metaLastError: err.Error()})
		status.LastError = err.Error()
		return status, err
	}
	status.FullRelease = feed.Full
	status.WeeklyReleases = feed.Weekly
	now := service.now()
	status.LastCheckAt = NowStamp(now)
	service.saveMeta(map[string]string{metaLastCheckAt: status.LastCheckAt, metaLastError: ""})
	if feed.Full != nil && status.LastFullDate != "" && feed.Full.Date > status.LastFullDate {
		status.FullDue = true
	}
	baseline := status.LastWeekEnd
	if baseline == "" {
		baseline = status.LastFullDate
	}
	if feed.Full != nil && status.LastFullDate == "" {
		status.NeedsFull = true
	}
	if len(feed.Weekly) > 0 {
		last := feed.Weekly[len(feed.Weekly)-1]
		status.WeeklyAvailable = last.End > baseline
	}
	return status, nil
}

func (service *Service) code() (string, error) {
	if service.Code == nil {
		return "", fmt.Errorf("统一社会信用代码读取失败")
	}
	code, err := service.Code()
	if err != nil {
		return "", err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("请先在参数设定中填写统一社会信用代码")
	}
	return code, nil
}

func (service *Service) fetchFeed(ctx context.Context) (*Feed, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.FeedURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "SsrNmpa/1.0")
	response, err := service.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("读取 NMPA RSS 失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取 NMPA RSS 失败: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 NMPA RSS 失败: %w", err)
	}
	return ParseFeed(body, service.FeedURL)
}

// SyncFull 下载最新全量包，流式筛选后替换参考库；不把全量 XML 落盘。
func (service *Service) SyncFull(ctx context.Context) (SyncResult, error) {
	status, err := service.Check(ctx)
	if err != nil {
		return SyncResult{Mode: "full", Status: status}, err
	}
	if status.FullRelease == nil {
		return SyncResult{Mode: "full", Status: status}, fmt.Errorf("RSS 中没有可用的全量包")
	}
	code := status.SocialCreditCode
	zipPath, err := service.download(ctx, *status.FullRelease)
	if err != nil {
		return SyncResult{Mode: "full", FileName: status.FullRelease.FileName, Status: status}, err
	}
	defer os.Remove(zipPath)
	service.progress(Progress{Stage: "parse"})
	parsed, err := ParseArchive(zipPath, code)
	if err != nil {
		return SyncResult{Mode: "full", FileName: status.FullRelease.FileName, Status: status}, err
	}
	now := service.now()
	values := map[string]string{
		metaCode:           code,
		metaFullFile:       status.FullRelease.FileName,
		metaFullDate:       status.FullRelease.Date,
		metaFullAt:         NowStamp(now),
		metaLastWeekEnd:    status.FullRelease.Date,
		metaLastWeeklyFile: "",
		metaLastSyncAt:     NowStamp(now),
		metaLastError:      "",
	}
	if err := service.Store.ReplaceAll(parsed.Devices, values); err != nil {
		return SyncResult{Mode: "full", FileName: status.FullRelease.FileName, Status: status}, err
	}
	status, _ = service.Status()
	result := SyncResult{
		Mode: "full", FileName: status.LastFullFile,
		Matched: len(parsed.Devices), Devices: status.DeviceCount,
		Packages: status.PackageCount, Contacts: status.ContactCount,
		Status: status,
	}
	if len(parsed.Devices) == 0 {
		result.Message = "全量包中没有匹配该统一社会信用代码的设备"
	} else {
		result.Message = fmt.Sprintf("全量同步完成，写入 %d 条设备", len(parsed.Devices))
	}
	return result, nil
}

// SyncWeekly 按顺序应用缺失的周更；发现中间断档时要求先做全量同步。
func (service *Service) SyncWeekly(ctx context.Context) (SyncResult, error) {
	status, err := service.Check(ctx)
	if err != nil {
		return SyncResult{Mode: "weekly", Status: status}, err
	}
	if status.CodeChanged {
		return SyncResult{Mode: "weekly", Status: status}, fmt.Errorf("统一社会信用代码已变化，请先执行全量同步")
	}
	if status.LastFullDate == "" {
		return SyncResult{Mode: "weekly", Status: status}, fmt.Errorf("参考库还没有全量数据，请先执行全量同步")
	}
	if status.FullDue {
		return SyncResult{Mode: "weekly", Status: status}, fmt.Errorf("有新的全量包，请先执行全量同步")
	}
	baseline := status.LastWeekEnd
	if baseline == "" {
		baseline = status.LastFullDate
	}
	baselineTime, err := time.Parse("2006-01-02", baseline)
	if err != nil {
		return SyncResult{Mode: "weekly", Status: status}, fmt.Errorf("本地周更基线无法解析: %s", baseline)
	}
	pending := make([]Release, 0)
	for _, release := range status.WeeklyReleases {
		if release.End > baseline {
			pending = append(pending, release)
		}
	}
	if len(pending) == 0 {
		status.NeedsFull = false
		return SyncResult{Mode: "weekly", Status: status, Message: "已经是最新周更"}, nil
	}
	if start, err := time.Parse("2006-01-02", pending[0].Start); err == nil {
		if start.After(baselineTime.AddDate(0, 0, 7)) {
			return SyncResult{Mode: "weekly", Status: status}, fmt.Errorf("周更出现断档（本地 %s，RSS 最早 %s），请先执行全量同步", baseline, pending[0].Start)
		}
	}
	totalMatched := 0
	lastFile := ""
	for _, release := range pending {
		zipPath, err := service.download(ctx, release)
		if err != nil {
			return SyncResult{Mode: "weekly", FileName: lastFile, Status: status}, err
		}
		parsed, parseErr := ParseArchive(zipPath, status.SocialCreditCode)
		os.Remove(zipPath)
		if parseErr != nil {
			return SyncResult{Mode: "weekly", FileName: release.FileName, Status: status}, parseErr
		}
		now := service.now()
		values := map[string]string{
			metaCode:           status.SocialCreditCode,
			metaLastWeekEnd:    release.End,
			metaLastWeeklyFile: release.FileName,
			metaLastSyncAt:     NowStamp(now),
			metaLastError:      "",
		}
		if err := service.Store.Upsert(parsed.Devices, values); err != nil {
			return SyncResult{Mode: "weekly", FileName: release.FileName, Status: status}, err
		}
		totalMatched += len(parsed.Devices)
		lastFile = release.FileName
		status.LastWeekEnd = release.End
		status.LastWeeklyFile = release.FileName
	}
	status, _ = service.Status()
	result := SyncResult{
		Mode: "weekly", FileName: lastFile, Matched: totalMatched,
		Devices: status.DeviceCount, Packages: status.PackageCount, Contacts: status.ContactCount,
		Status: status,
	}
	if totalMatched == 0 {
		result.Message = "周更包中没有匹配该统一社会信用代码的设备"
	} else {
		result.Message = fmt.Sprintf("周更同步完成，应用 %d 个发布包，写入/更新 %d 条设备", len(pending), totalMatched)
	}
	return result, nil
}

// Query 查询本地参考库。
func (service *Service) Query(query string, limit, offset int) (QueryResult, error) {
	if service == nil || service.Store == nil {
		return QueryResult{}, fmt.Errorf("药监参考库未初始化")
	}
	return service.Store.Search(query, limit, offset)
}

func (service *Service) download(ctx context.Context, release Release) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "SsrNmpa/1.0")
	response, err := service.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("下载 NMPA 发布包失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 NMPA 发布包失败: HTTP %d", response.StatusCode)
	}
	file, err := os.CreateTemp("", "ssr-nmpa-*.zip")
	if err != nil {
		return "", fmt.Errorf("创建 NMPA 临时文件失败: %w", err)
	}
	path := file.Name()
	defer file.Close()
	reader := io.Reader(response.Body)
	if response.ContentLength > 0 {
		reader = &progressReader{reader: response.Body, total: response.ContentLength, service: service}
	}
	if _, err := io.Copy(file, reader); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("下载 NMPA 发布包失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("保存 NMPA 发布包失败: %w", err)
	}
	return path, nil
}

type progressReader struct {
	reader  io.Reader
	total   int64
	loaded  int64
	service *Service
}

func (reader *progressReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.loaded += int64(count)
	reader.service.progress(Progress{Stage: "download", Loaded: reader.loaded, Total: reader.total})
	return count, err
}

func (service *Service) client() *http.Client {
	if service.Client != nil {
		return service.Client
	}
	return &http.Client{Timeout: 0}
}

func (service *Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service *Service) progress(progress Progress) {
	if service.OnProgress != nil {
		service.OnProgress(progress)
	}
}

func (service *Service) saveMeta(values map[string]string) {
	if service.Store == nil {
		return
	}
	_ = service.Store.SetMetadata(values)
}

// ResolveDownloadURL 处理 RSS 中偶尔出现的相对链接。
func ResolveDownloadURL(baseURL, link string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(parsed).String(), nil
}

// DatabaseDirectory 返回参考库所在目录（给界面显示/打开目录用）。
func DatabaseDirectory(path string) string {
	return filepath.Dir(path)
}
