// Package update 检测 frpc-mesh 自身在 GitHub Releases 上是否有新版本。
// 只做检测与提示，不替换运行中的二进制（Docker 场景下替换会被镜像覆盖）。
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"frpc-mesh/internal/store"
)

const (
	// Repo 本项目仓库， releases/latest 取最新正式版
	repo    = "busfly/frpc-mesh"
	latest  = "https://api.github.com/repos/" + repo + "/releases/latest"
	mirror  = "https://ghfast.top/"
	setting = "update_check"
	// AutoInterval 自动检查周期；StartupDelay 启动后首次自动检查的延迟
	AutoInterval  = 6 * time.Hour
	StartupDelay  = 20 * time.Second
	maxNotesBytes = 4096
)

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type Asset struct {
	Name string `json:"name"`
	Size int64  `json:"size_bytes"`
	URL  string `json:"url"`
}

// Info 一次检查结果；CheckedAt 为空表示从未检查过
type Info struct {
	Current     string  `json:"current"`
	Latest      string  `json:"latest"`
	HasUpdate   bool    `json:"has_update"`
	ReleaseName string  `json:"release_name"`
	ReleaseURL  string  `json:"release_url"`
	PublishedAt string  `json:"published_at"`
	Notes       string  `json:"notes"`
	Assets      []Asset `json:"assets"`
	CheckedAt   string  `json:"checked_at"`
	Source      string  `json:"source"`
	Error       string  `json:"error,omitempty"`
	Auto        bool    `json:"auto_check"`
}

type remoteRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Assets      []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type Checker struct {
	mu        sync.Mutex
	store     *store.Store
	current   string
	client    *http.Client
	apiURL    string
	mirrorURL string
	last      *Info
}

func New(st *store.Store, current string) *Checker {
	return &Checker{
		store:     st,
		current:   current,
		client:    &http.Client{Timeout: 20 * time.Second},
		apiURL:    latest,
		mirrorURL: mirror + latest,
	}
}

// Status 返回本地已知的检查结果，不访问网络（秒回，供首屏渲染）
func (c *Checker) Status() *Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		c.last = c.load()
	}
	inf := *c.last
	inf.Current = c.current
	inf.Auto = c.autoEnabled()
	inf.HasUpdate = hasUpdate(c.current, inf.Latest)
	return &inf
}

// Result 强制联网检查：先直连 GitHub API，失败再走镜像；结果落盘，重启后仍在
func (c *Checker) Result(source string) *Info {
	inf := &Info{Current: c.current, CheckedAt: time.Now().Format(time.RFC3339)}
	inf.Auto = c.autoEnabled()

	var err error
	if source == "ghfast" {
		err = c.fetch(c.mirrorURL, inf)
		inf.Source = source
	} else {
		inf.Source = "github"
		err = c.fetch(c.apiURL, inf)
		if err != nil {
			inf.Source = "ghfast"
			err = c.fetch(c.mirrorURL, inf)
		}
	}
	if err != nil {
		inf.Error = err.Error()
	}
	c.mu.Lock()
	c.last = inf
	c.mu.Unlock()
	c.save(inf)
	return c.Status()
}

func (c *Checker) fetch(url string, inf *Info) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "frpc-mesh")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求 GitHub 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub 返回状态 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}
	var rel remoteRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return fmt.Errorf("解析 Release 失败: %v", err)
	}
	ver := strings.TrimPrefix(rel.TagName, "v")
	if !versionRe.MatchString(ver) {
		return fmt.Errorf("未能识别最新版本号 %q", rel.TagName)
	}
	inf.Latest = ver
	inf.ReleaseName = rel.Name
	inf.ReleaseURL = rel.HTMLURL
	inf.PublishedAt = rel.PublishedAt
	inf.Notes = strings.TrimSpace(truncate(rel.Body, maxNotesBytes))
	inf.Error = ""
	for _, a := range rel.Assets {
		inf.Assets = append(inf.Assets, Asset{Name: a.Name, Size: a.Size, URL: a.BrowserDownloadURL})
	}
	sort.Slice(inf.Assets, func(i, j int) bool { return inf.Assets[i].Name < inf.Assets[j].Name })
	return nil
}

// Loop 周期性自动检查；开关关闭时跳过检查但不退出，便于随时再打开
func (c *Checker) Loop(stop <-chan struct{}) {
	select {
	case <-time.After(StartupDelay):
	case <-stop:
		return
	}
	for {
		if c.autoEnabled() {
			c.Result("github")
		}
		select {
		case <-time.After(AutoInterval):
		case <-stop:
			return
		}
	}
}

func (c *Checker) autoEnabled() bool {
	return c.store.GetSettings().AutoUpdateCheck
}

func (c *Checker) load() *Info {
	raw := c.store.GetSetting(setting, "")
	if raw == "" {
		return &Info{}
	}
	var inf Info
	if err := json.Unmarshal([]byte(raw), &inf); err != nil {
		return &Info{}
	}
	return &inf
}

// save 只持久化远程结果与检查时间，当前版本与开关状态读取时现算
func (c *Checker) save(inf *Info) {
	b, err := json.Marshal(struct {
		Latest      string  `json:"latest"`
		ReleaseName string  `json:"release_name"`
		ReleaseURL  string  `json:"release_url"`
		PublishedAt string  `json:"published_at"`
		Notes       string  `json:"notes"`
		Assets      []Asset `json:"assets"`
		CheckedAt   string  `json:"checked_at"`
		Source      string  `json:"source"`
		Error       string  `json:"error,omitempty"`
	}{
		Latest: inf.Latest, ReleaseName: inf.ReleaseName, ReleaseURL: inf.ReleaseURL,
		PublishedAt: inf.PublishedAt, Notes: inf.Notes, Assets: inf.Assets,
		CheckedAt: inf.CheckedAt, Source: inf.Source, Error: inf.Error,
	})
	if err != nil {
		return
	}
	c.store.SetSetting(setting, string(b))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// hasUpdate 比较版本号：仅当两者都是 x.y.z 且 latest 更大时为 true
func hasUpdate(current, latestVer string) bool {
	cur := strings.TrimPrefix(current, "v")
	if !versionRe.MatchString(cur) || !versionRe.MatchString(latestVer) {
		return false
	}
	return compare(cur, latestVer) < 0
}

func compare(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}
