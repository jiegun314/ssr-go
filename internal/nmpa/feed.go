package nmpa

import (
	"encoding/xml"
	"fmt"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FeedURL 是药监 UDI 发布文件的 RSS 索引。
const FeedURL = "https://udi.nmpa.gov.cn/rss/download.html?files=all"

// Feed 是解析后的发布索引。
type Feed struct {
	Title  string
	Items  []Release
	Full   *Release
	Weekly []Release
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

var releaseNamePattern = regexp.MustCompile(`^UDID_(DAY|WEEKLY|MONTH|FULL)_(?:UPDATE|RELEASE)_([0-9]{6,8}(?:_[0-9]{8})?)\.zip$`)
var countPattern = regexp.MustCompile(`([0-9][0-9,]*)\s*(?:条|个|件|产品标识)`)

// ParseFeed 解析 RSS，并把发布包按频率分类、按日期排序。
func ParseFeed(data []byte, sourceURL string) (*Feed, error) {
	var document rssDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse NMPA RSS: %w", err)
	}
	base, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("parse NMPA RSS URL: %w", err)
	}
	feed := &Feed{Title: strings.TrimSpace(document.Channel.Title)}
	for _, item := range document.Channel.Items {
		release, ok := releaseFromItem(item, base)
		if !ok {
			continue
		}
		feed.Items = append(feed.Items, release)
	}
	sort.SliceStable(feed.Items, func(i, j int) bool {
		if feed.Items[i].PublishedAt == feed.Items[j].PublishedAt {
			return feed.Items[i].FileName > feed.Items[j].FileName
		}
		return feed.Items[i].PublishedAt > feed.Items[j].PublishedAt
	})
	for index := range feed.Items {
		release := &feed.Items[index]
		switch release.Kind {
		case KindFull:
			if feed.Full == nil || release.Date > feed.Full.Date {
				feed.Full = release
			}
		case KindWeekly:
			feed.Weekly = append(feed.Weekly, *release)
		}
	}
	sort.SliceStable(feed.Weekly, func(i, j int) bool { return feed.Weekly[i].End < feed.Weekly[j].End })
	return feed, nil
}

func releaseFromItem(item rssItem, base *url.URL) (Release, bool) {
	link := strings.TrimSpace(item.Link)
	if link == "" {
		link = strings.TrimSpace(item.GUID)
	}
	parsedLink, err := url.Parse(link)
	if err != nil {
		return Release{}, false
	}
	resolved := base.ResolveReference(parsedLink)
	name := path.Base(resolved.Path)
	// 真实 RSS 的链接统一指向 download.html，文件名只出现在 title 中；
	// 测试与旧格式则可能直接把文件名放在链接路径上。
	if !releaseNamePattern.MatchString(name) {
		name = strings.TrimSpace(item.Title)
	}
	match := releaseNamePattern.FindStringSubmatch(name)
	if len(match) != 3 {
		return Release{}, false
	}
	kind := KindDaily
	datePart := match[2]
	releaseDate := ""
	start, end := "", ""
	switch match[1] {
	case "FULL":
		kind = KindFull
		releaseDate = parseCompactDate(datePart)
	case "MONTH":
		kind = KindMonthly
		releaseDate = parseCompactMonth(datePart)
	case "WEEKLY":
		kind = KindWeekly
		parts := strings.Split(datePart, "_")
		if len(parts) == 2 {
			start = parseCompactDate(parts[0])
			end = parseCompactDate(parts[1])
			releaseDate = end
		}
	case "DAY":
		kind = KindDaily
		releaseDate = parseCompactDate(datePart)
	}
	publishedAt := ""
	if parsed, err := mail.ParseDate(strings.TrimSpace(item.PubDate)); err == nil {
		publishedAt = parsed.Format(time.RFC3339)
	}
	return Release{
		Kind:        kind,
		FileName:    name,
		Title:       strings.TrimSpace(item.Title),
		URL:         resolved.String(),
		PublishedAt: publishedAt,
		Date:        releaseDate,
		Start:       start,
		End:         end,
		Count:       parseCount(item.Description),
	}, true
}

func parseCompactDate(value string) string {
	if len(value) != 8 {
		return ""
	}
	parsed, err := time.Parse("20060102", value)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02")
}

func parseCompactMonth(value string) string {
	if len(value) != 6 {
		return ""
	}
	if _, err := time.Parse("200601", value); err != nil {
		return ""
	}
	return value[:4] + "-" + value[4:]
}

func parseCount(description string) int {
	match := countPattern.FindStringSubmatch(description)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return 0
	}
	return value
}
