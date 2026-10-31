package runner

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type VersionInfo struct {
	Version       string `json:"version"`
	SizeBytes     int64  `json:"size_bytes"`
	DownloadCount int64  `json:"download_count"`
	PublishedAt   string `json:"published_at"`
	Downloaded    bool   `json:"downloaded"`
	Active        bool   `json:"active"`
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Assets     []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		DownloadCount      int64  `json:"download_count"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (m *Manager) VersionsDir() string {
	d := filepath.Join(m.dataDir, "versions")
	os.MkdirAll(d, 0o755)
	return d
}

func (m *Manager) versionPath(version string) string {
	return filepath.Join(m.VersionsDir(), "frpc-"+version)
}

func (m *Manager) ActiveVersion() string {
	return m.store.GetSetting("frpc_version", "")
}

func (m *Manager) ListLocalVersions() []string {
	entries, err := os.ReadDir(m.VersionsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "frpc-") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		ver := strings.TrimPrefix(name, "frpc-")
		if versionRe.MatchString(ver) {
			out = append(out, ver)
		}
	}
	return out
}

// CleanTempFiles 清理中断下载残留的 .tmp 文件
func (m *Manager) CleanTempFiles() {
	entries, err := os.ReadDir(m.VersionsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			os.Remove(filepath.Join(m.VersionsDir(), e.Name()))
		}
	}
}

func (m *Manager) HasVersion(version string) bool {
	fi, err := os.Stat(m.versionPath(version))
	return err == nil && !fi.IsDir()
}

// ResolveFrpc 优先使用已激活的下载版本，其次系统 PATH 与常见位置探测
func (m *Manager) resolveFrpcBinary() (string, error) {
	if v := m.ActiveVersion(); v != "" && m.HasVersion(v) {
		return m.versionPath(v), nil
	}
	if p, err := exec.LookPath("frpc"); err == nil {
		return p, nil
	}
	for _, cand := range []string{
		"/usr/local/bin/frpc", "/usr/bin/frpc", "/opt/frpc/frpc",
		filepath.Join(m.dataDir, "bin", "frpc"), "./frpc",
	} {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf("未找到 frpc 二进制，请在设置中指定路径或在版本管理页下载")
}

// LocalVersions 仅返回本地已下载版本（秒回，不访问网络）
func (m *Manager) LocalVersions() []VersionInfo {
	active := m.ActiveVersion()
	var out []VersionInfo
	for _, v := range m.ListLocalVersions() {
		out = append(out, VersionInfo{Version: v, Downloaded: true, Active: v == active})
	}
	sortVersions(out)
	return out
}

type remoteCacheEntry struct {
	at   time.Time
	list []VersionInfo
}

// FetchRemoteVersionsCached 带 10 分钟缓存的远程版本扫描
func (m *Manager) FetchRemoteVersionsCached(source string) ([]VersionInfo, error) {
	m.mu.Lock()
	if m.remoteCache == nil {
		m.remoteCache = map[string]remoteCacheEntry{}
	}
	if c, ok := m.remoteCache[source]; ok && time.Since(c.at) < 10*time.Minute {
		m.mu.Unlock()
		return c.list, nil
	}
	m.mu.Unlock()
	list, err := m.FetchRemoteVersions(source)
	if err == nil {
		m.mu.Lock()
		m.remoteCache[source] = remoteCacheEntry{at: time.Now(), list: list}
		m.mu.Unlock()
	}
	return list, err
}

// MergeVersions 以远程列表为基础，补充仅本地存在的版本，按版本号降序
func (m *Manager) MergeVersions(remote []VersionInfo) []VersionInfo {
	local := map[string]bool{}
	for _, v := range m.ListLocalVersions() {
		local[v] = true
	}
	out := make([]VersionInfo, 0, len(remote)+len(local))
	seen := map[string]bool{}
	active := m.ActiveVersion()
	for _, r := range remote {
		r.Downloaded = local[r.Version]
		r.Active = r.Version == active
		out = append(out, r)
		seen[r.Version] = true
	}
	for v := range local {
		if !seen[v] {
			out = append(out, VersionInfo{Version: v, Downloaded: true, Active: v == active})
		}
	}
	sortVersions(out)
	return out
}

func sortVersions(list []VersionInfo) {
	sort.Slice(list, func(i, j int) bool {
		return compareVersions(list[i].Version, list[j].Version) > 0
	})
}

func compareVersions(a, b string) int {
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

// FetchRemoteVersions 扫描 GitHub Releases 获取可用版本
func (m *Manager) FetchRemoteVersions(source string) ([]VersionInfo, error) {
	apiURL := "https://api.github.com/repos/fatedier/frp/releases?per_page=30"
	if source == "ghfast" {
		apiURL = "https://ghfast.top/" + apiURL
	}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "frpc-mesh")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == 200 {
		if list, ok := parseReleases(resp.Body, m); ok {
			resp.Body.Close()
			return list, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}
	return m.fetchTagsFallback()
}

func parseReleases(body io.Reader, m *Manager) ([]VersionInfo, bool) {
	var releases []ghRelease
	if err := json.NewDecoder(body).Decode(&releases); err != nil {
		return nil, false
	}
	assetName := fmt.Sprintf("frp_v%s_%s_%s.tar.gz", "%s", runtime.GOOS, runtime.GOARCH)
	active := m.ActiveVersion()
	local := map[string]bool{}
	for _, v := range m.ListLocalVersions() {
		local[v] = true
	}
	var out []VersionInfo
	for _, rel := range releases {
		ver := strings.TrimPrefix(rel.TagName, "v")
		if !versionRe.MatchString(ver) {
			continue
		}
		want := fmt.Sprintf(assetName, ver)
		for _, a := range rel.Assets {
			if a.Name == want {
				out = append(out, VersionInfo{
					Version: ver, SizeBytes: a.Size, DownloadCount: a.DownloadCount,
					PublishedAt: rel.PublishedAt, Downloaded: local[ver], Active: ver == active,
				})
				break
			}
		}
	}
	return out, len(out) > 0
}

// fetchTagsFallback 在 GitHub API 不可达时抓取 tags 页面提取版本号（无元数据）
func (m *Manager) fetchTagsFallback() ([]VersionInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://github.com/fatedier/frp/tags")
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`releases/tag/v(\d+\.\d+\.\d+)`)
	matches := re.FindAllStringSubmatch(string(body), -1)
	active := m.ActiveVersion()
	local := map[string]bool{}
	for _, v := range m.ListLocalVersions() {
		local[v] = true
	}
	seen := map[string]bool{}
	var out []VersionInfo
	for _, mt := range matches {
		ver := mt[1]
		if seen[ver] {
			continue
		}
		seen[ver] = true
		out = append(out, VersionInfo{Version: ver, Downloaded: local[ver], Active: ver == active})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未能解析版本列表")
	}
	return out, nil
}

// DownloadVersion 下载并解压指定版本的 frpc 二进制
func (m *Manager) DownloadVersion(version, source string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("非法的版本号: %s", version)
	}
	url := fmt.Sprintf("https://github.com/fatedier/frp/releases/download/v%s/frp_%s_%s_%s.tar.gz",
		version, version, runtime.GOOS, runtime.GOARCH)
	if source == "ghfast" {
		url = "https://ghfast.top/" + url
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载返回状态 %d，请检查版本是否存在或切换镜像源", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}
	defer gz.Close()
	tmp := m.versionPath(version) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	found := false
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("读取压缩包失败: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "frpc" {
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			found = true
			break
		}
	}
	f.Close()
	if !found {
		os.Remove(tmp)
		return fmt.Errorf("压缩包中未找到 frpc 二进制")
	}
	out, err := exec.Command(tmp, "-v").Output()
	if err != nil || strings.TrimSpace(string(out)) != version {
		os.Remove(tmp)
		return fmt.Errorf("下载的二进制校验失败（报告版本 %q）", strings.TrimSpace(string(out)))
	}
	return os.Rename(tmp, m.versionPath(version))
}

// ---------- 异步下载任务 ----------

type downloadJob struct {
	mu      sync.Mutex
	status  string // downloading | done | error
	message string
}

func (m *Manager) StartDownload(version, source string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("非法的版本号: %s", version)
	}
	m.mu.Lock()
	if m.jobs == nil {
		m.jobs = map[string]*downloadJob{}
	}
	if j, ok := m.jobs[version]; ok {
		j.mu.Lock()
		running := j.status == "downloading"
		j.mu.Unlock()
		if running {
			m.mu.Unlock()
			return fmt.Errorf("版本 %s 正在下载中", version)
		}
	}
	job := &downloadJob{status: "downloading"}
	m.jobs[version] = job
	m.mu.Unlock()

	go func() {
		err := m.DownloadVersion(version, source)
		job.mu.Lock()
		if err != nil {
			job.status = "error"
			job.message = err.Error()
		} else {
			job.status = "done"
		}
		job.mu.Unlock()
	}()
	return nil
}

func (m *Manager) DownloadStatus(version string) (string, string) {
	m.mu.Lock()
	j := m.jobs[version]
	m.mu.Unlock()
	if j == nil {
		return "none", ""
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status, j.message
}

func (m *Manager) DeleteVersion(version string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("非法的版本号: %s", version)
	}
	if version == m.ActiveVersion() {
		return fmt.Errorf("不能删除正在使用的版本")
	}
	if st, _ := m.DownloadStatus(version); st == "downloading" {
		return fmt.Errorf("版本 %s 正在下载中，无法删除", version)
	}
	return os.Remove(m.versionPath(version))
}

// ActivateVersion 切换生效版本并重启所有运行中实例
func (m *Manager) ActivateVersion(version string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("非法的版本号: %s", version)
	}
	if !m.HasVersion(version) {
		return fmt.Errorf("版本 %s 尚未下载", version)
	}
	if err := m.store.SetSetting("frpc_version", version); err != nil {
		return err
	}
	m.RestartAllRunning()
	return nil
}

func (m *Manager) RestartAllRunning() {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Restart(id)
	}
}
