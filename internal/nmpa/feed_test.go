package nmpa

import "testing"

func TestParseFeedClassifiesAndSortsReleases(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>医疗器械唯一标识数据库发布文件</title>
<item><title>全量</title><link>https://udi.nmpa.gov.cn/rss/UDID_FULL_RELEASE_20261101.zip</link><description>共 6,400,134 条</description><pubDate>Fri, 31 Oct 2026 17:48:00 +0000</pubDate></item>
<item><title>周更</title><link>https://udi.nmpa.gov.cn/rss/UDID_WEEKLY_UPDATE_20261116_20261122.zip</link><description>54,227 条</description><pubDate>Sun, 22 Nov 2026 18:05:00 +0000</pubDate></item>
<item><title>周更旧</title><link>https://udi.nmpa.gov.cn/rss/UDID_WEEKLY_UPDATE_20261109_20261115.zip</link><description>96,967 条</description><pubDate>Sun, 15 Nov 2026 18:05:00 +0000</pubDate></item>
<item><title>月更</title><link>https://udi.nmpa.gov.cn/rss/UDID_MONTH_UPDATE_202610.zip</link><description>215,401 条</description><pubDate>Sat, 01 Nov 2026 17:48:00 +0000</pubDate></item>
<item><title>日更</title><link>https://udi.nmpa.gov.cn/rss/UDID_DAY_UPDATE_20261123.zip</link><description>1,605 条</description><pubDate>Sun, 23 Nov 2026 15:35:00 +0000</pubDate></item>
<item><title>无关</title><link>https://udi.nmpa.gov.cn/rss/other.zip</link><description>0 条</description></item>
</channel></rss>`)
	feed, err := ParseFeed(data, "https://udi.nmpa.gov.cn/rss/download.html?files=all")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Title != "医疗器械唯一标识数据库发布文件" {
		t.Fatalf("title = %q", feed.Title)
	}
	if len(feed.Items) != 5 {
		for _, item := range feed.Items {
			t.Logf("item: %s %s", item.Kind, item.FileName)
		}
		t.Fatalf("items = %d, want 5", len(feed.Items))
	}
	if feed.Full == nil || feed.Full.FileName != "UDID_FULL_RELEASE_20261101.zip" || feed.Full.Date != "2026-11-01" {
		t.Fatalf("full = %#v", feed.Full)
	}
	if feed.Full.Count != 6400134 {
		t.Fatalf("full count = %d", feed.Full.Count)
	}
	if len(feed.Weekly) != 2 || feed.Weekly[0].Start != "2026-11-09" || feed.Weekly[1].End != "2026-11-22" {
		t.Fatalf("weekly = %#v", feed.Weekly)
	}
	if feed.Weekly[0].Count != 96967 || feed.Weekly[1].Count != 54227 {
		t.Fatalf("weekly counts = %d/%d", feed.Weekly[0].Count, feed.Weekly[1].Count)
	}
	var monthly *Release
	for index := range feed.Items {
		if feed.Items[index].Kind == KindMonthly {
			monthly = &feed.Items[index]
		}
	}
	if monthly == nil || monthly.Date != "2026-10" || monthly.Count != 215401 {
		t.Fatalf("monthly = %#v", monthly)
	}
	if feed.Items[0].PublishedAt == "" {
		t.Fatal("publishedAt should be normalized")
	}
}

func TestParseFeedRejectsGarbage(t *testing.T) {
	if _, err := ParseFeed([]byte("not rss"), FeedURL); err == nil {
		t.Fatal("expected parse error")
	}
}
