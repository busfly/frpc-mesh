package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"frpc-mesh/internal/model"
	"frpc-mesh/internal/store"
	"frpc-mesh/internal/tomlegen"
)

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusOnline   = "online"
	StatusError    = "error"
	StatusStopping = "stopping"

	maxLogLines = 1000
)

type proc struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	status    string
	lastError string
	startedAt time.Time
	lines     []string
	online    bool
	// frpcVersion 启动时实际使用的版本，创建后只读；空串=系统 frpc 兜底（版本未知）
	frpcVersion string
	proxySt     map[string]string // 代理名 → running | error | pending（源自 frpc 日志）
	done        chan struct{}     // wait goroutine 退出时关闭
}

func (p *proc) setProxyStatus(name, st string) {
	if name == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proxySt == nil {
		p.proxySt = map[string]string{}
	}
	p.proxySt[name] = st
}

func (p *proc) resetProxies(names []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := make(map[string]string, len(names))
	for _, n := range names {
		m[n] = "pending"
	}
	p.proxySt = m
}

func (p *proc) appendLine(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lines = append(p.lines, line)
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}
}

func (p *proc) snapshot() (string, string, time.Time, []string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.lines))
	copy(out, p.lines)
	pid := 0
	if p.cmd != nil && p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
	}
	return p.status, p.lastError, p.startedAt, out, pid
}

func (p *proc) setStatus(status, lastErr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
	if lastErr != "" {
		p.lastError = lastErr
	}
	if status == StatusOnline {
		p.online = true
	}
}

type Manager struct {
	mu          sync.Mutex
	procs       map[int64]*proc
	jobs        map[string]*downloadJob
	remoteCache map[string]remoteCacheEntry
	fails       map[int64]int // 连续快速退出计数（自愈熔断用）
	// downloadFailed 记录本进程内补下载已失败的 pin 版本：自动 reconcile 不再反复重试，
	// 用户重新保存该服务器时清除标记（显式动作即重试意图）
	downloadFailed map[string]struct{}
	// downloadFunc / restartFunc / watchPoll 为可注入项，单测替换以隔离真实网络与进程（同 update 包 execFunc 套路）
	downloadFunc func(version, source string) error
	restartFunc  func(id int64) error
	watchPoll    time.Duration
	store        *store.Store
	dataDir      string
}

func NewManager(st *store.Store, dataDir string) *Manager {
	m := &Manager{
		procs: map[int64]*proc{}, jobs: map[string]*downloadJob{}, remoteCache: map[string]remoteCacheEntry{},
		fails: map[int64]int{}, downloadFailed: map[string]struct{}{}, store: st, dataDir: dataDir,
		watchPoll: 2 * time.Second,
	}
	m.downloadFunc = m.StartDownload
	m.restartFunc = m.Restart
	return m
}

func (m *Manager) ConfigDir() string {
	d := filepath.Join(m.dataDir, "configs")
	os.MkdirAll(d, 0o755)
	return d
}

// ResolveFrpc 返回可用的 frpc 二进制路径
func (m *Manager) ResolveFrpc() (string, error) {
	return m.resolveFrpcBinary()
}

// ResolveFrpcFor 按实例解析 frpc 二进制与生效版本：pin 已下载 > 默认版本已下载 > 系统兜底。
// effective 为空串表示走的是系统 frpc 兜底，版本未知。
func (m *Manager) ResolveFrpcFor(in *model.Instance) (path, effective string, err error) {
	if in != nil && in.FrpcVersion != "" && m.HasVersion(in.FrpcVersion) {
		return m.versionPath(in.FrpcVersion), in.FrpcVersion, nil
	}
	if v := m.ActiveVersion(); v != "" && m.HasVersion(v) {
		return m.versionPath(v), v, nil
	}
	p, err := m.resolveFrpcBinary()
	return p, "", err
}

func (m *Manager) FrpcVersion() string {
	p, err := m.ResolveFrpc()
	if err != nil {
		return ""
	}
	out, err := exec.Command(p, "-v").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *Manager) GenerateConfig(in *model.Instance) (string, string, error) {
	proxies, err := m.store.ListProxies(in.ID)
	if err != nil {
		return "", "", err
	}
	content := tomlegen.Generate(in, activeProxies(proxies))
	path := filepath.Join(m.ConfigDir(), fmt.Sprintf("instance-%d.toml", in.ID))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", "", err
	}
	return path, content, nil
}

// activeProxies 禁用的代理不写入 frpc 配置
func activeProxies(proxies []*model.Proxy) []*model.Proxy {
	out := make([]*model.Proxy, 0, len(proxies))
	for _, p := range proxies {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

func (m *Manager) PreviewConfig(id int64) (string, error) {
	in, err := m.store.GetInstance(id)
	if err != nil {
		return "", err
	}
	proxies, err := m.store.ListProxies(in.ID)
	if err != nil {
		return "", err
	}
	return tomlegen.Generate(in, activeProxies(proxies)), nil
}

func (m *Manager) Runtime(id int64) *model.Runtime {
	m.mu.Lock()
	p := m.procs[id]
	m.mu.Unlock()
	rt := &model.Runtime{Status: StatusStopped}
	if p == nil {
		return rt
	}
	status, lastErr, startedAt, _, pid := p.snapshot()
	rt.Status = status
	rt.LastError = lastErr
	rt.PID = pid
	rt.FrpcVersion = p.frpcVersion
	if !startedAt.IsZero() {
		rt.StartedAt = startedAt.UTC().Format(time.RFC3339)
		rt.UptimeSeconds = int64(time.Since(startedAt).Seconds())
	}
	return rt
}

func (m *Manager) ClearLogs(id int64) {
	m.mu.Lock()
	p := m.procs[id]
	m.mu.Unlock()
	if p == nil {
		return
	}
	p.mu.Lock()
	p.lines = nil
	p.mu.Unlock()
}

func (m *Manager) Logs(id int64, tail int) []string {
	m.mu.Lock()
	p := m.procs[id]
	m.mu.Unlock()
	if p == nil {
		return []string{}
	}
	_, _, _, lines, _ := p.snapshot()
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines
}

func (m *Manager) IsRunning(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.procs[id]
	if p == nil {
		return false
	}
	s, _, _, _, _ := p.snapshot()
	return s != StatusStopped && s != StatusError
}

func (m *Manager) Start(in *model.Instance) error {
	m.mu.Lock()
	if p, ok := m.procs[in.ID]; ok {
		s, _, _, _, _ := p.snapshot()
		if s != StatusStopped && s != StatusError {
			m.mu.Unlock()
			return fmt.Errorf("实例已在运行中")
		}
		delete(m.procs, in.ID)
	}
	m.mu.Unlock()

	frpcPath, effectiveVersion, err := m.ResolveFrpcFor(in)
	if err != nil {
		return err
	}
	cfgPath, _, err := m.GenerateConfig(in)
	if err != nil {
		return err
	}
	if in.AdminCfg.Enabled && in.AdminCfg.Port > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", in.AdminCfg.Port))
		if err != nil {
			return fmt.Errorf("Admin API 端口 %d 被占用（可能被其他实例使用）", in.AdminCfg.Port)
		}
		ln.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, frpcPath, "-c", cfgPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("启动 frpc 失败: %v", err)
	}

	p := &proc{cmd: cmd, cancel: cancel, status: StatusStarting, startedAt: time.Now(), done: make(chan struct{}), frpcVersion: effectiveVersion}
	m.mu.Lock()
	m.procs[in.ID] = p
	m.mu.Unlock()

	go m.pump(in.ID, p, stdout)
	go m.pump(in.ID, p, stderr)
	go m.wait(in.ID, p, cmd)
	return nil
}

func (m *Manager) pump(id int64, p *proc, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := stripANSI(sc.Text())
		p.appendLine(line)
		m.classify(p, line)
	}
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiSeq.ReplaceAllString(s, "")
}

func (m *Manager) classify(p *proc, line string) {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "login to server success"):
		p.setStatus(StatusOnline, "")
	case strings.Contains(l, "start proxy success"):
		p.setProxyStatus(bracketNameBefore(line, "start proxy success"), "running")
	case strings.Contains(l, "start error:"):
		p.setProxyStatus(bracketNameBefore(line, "start error:"), "error")
	case strings.Contains(l, "proxy added:"):
		p.resetProxies(proxyAddedNames(line))
	case strings.Contains(l, "start admin server"):
		p.mu.Lock()
		if p.status == StatusStarting {
			p.status = StatusRunning
		}
		p.mu.Unlock()
	case strings.Contains(l, "connect to server error"),
		strings.Contains(l, "login to server failed"),
		strings.Contains(l, "failed to connect"):
		p.setStatus(StatusError, extractMsg(line))
	case strings.Contains(l, "address already in use"):
		p.setStatus(StatusError, extractMsg(line))
	case strings.Contains(l, "[e]"), strings.Contains(l, " error"):
		p.mu.Lock()
		if p.status != StatusOnline {
			p.lastError = extractMsg(line)
		}
		p.mu.Unlock()
	}
}

func extractMsg(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 300 {
		line = line[:300]
	}
	return line
}

// bracketNameBefore 取 suffix 之前最近一对中括号内的代理名，如 "... [macmini-hub] start proxy success"
func bracketNameBefore(line, suffix string) string {
	i := strings.Index(line, suffix)
	if i < 0 {
		return ""
	}
	head := line[:i]
	j := strings.LastIndex(head, "[")
	k := strings.LastIndex(head, "]")
	if j < 0 || k < j {
		return ""
	}
	return head[j+1 : k]
}

// proxyAddedNames 解析 "proxy added: [a b c]" 中的代理名列表
func proxyAddedNames(line string) []string {
	i := strings.Index(line, "proxy added:")
	if i < 0 {
		return nil
	}
	rest := strings.TrimSpace(line[i+len("proxy added:"):])
	rest = strings.TrimPrefix(rest, "[")
	rest = strings.TrimSuffix(rest, "]")
	if strings.TrimSpace(rest) == "" {
		return nil
	}
	return strings.Fields(rest)
}

// ProxyRunningNames 返回 frpc 日志确认隧道建立成功（running）的代理名集合
func (m *Manager) ProxyRunningNames(id int64) map[string]bool {
	m.mu.Lock()
	p := m.procs[id]
	m.mu.Unlock()
	out := map[string]bool{}
	if p == nil {
		return out
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for n, st := range p.proxySt {
		if st == "running" {
			out[n] = true
		}
	}
	return out
}

func (m *Manager) wait(id int64, p *proc, cmd *exec.Cmd) {
	defer close(p.done)
	err := cmd.Wait()
	p.mu.Lock()
	unexpected := p.status != StatusStopping
	if unexpected {
		p.status = StatusStopped
		if err != nil && p.lastError == "" {
			p.lastError = err.Error()
		}
	} else {
		p.status = StatusStopped
	}
	p.mu.Unlock()
	m.mu.Lock()
	if cur, ok := m.procs[id]; ok && cur == p {
		delete(m.procs, id)
	}
	m.mu.Unlock()
	if unexpected {
		m.autoRestart(id, p)
	}
}

// autoRestart 进程意外退出时自动恢复；连续快速退出（<15s）超过 3 次则停止自愈，避免崩溃循环
func (m *Manager) autoRestart(id int64, p *proc) {
	lived := time.Since(p.startedAt)
	m.mu.Lock()
	if lived > 15*time.Second {
		m.fails[id] = 0
	} else {
		m.fails[id]++
	}
	n := m.fails[id]
	m.mu.Unlock()
	if n > 3 {
		msg := "进程连续异常退出，已停止自动重启，请检查配置或日志后手动重启"
		tomb := &proc{status: StatusError, lastError: msg, done: make(chan struct{})}
		close(tomb.done)
		m.mu.Lock()
		if cur, ok := m.procs[id]; !ok || cur == p {
			m.procs[id] = tomb
		}
		m.mu.Unlock()
		return
	}
	go func() {
		time.Sleep(time.Duration(n) * 2 * time.Second)
		in, err := m.store.GetInstance(id)
		if err != nil || !in.Enabled {
			return
		}
		_ = m.Start(in)
	}()
}

func (m *Manager) Stop(id int64) error {
	m.mu.Lock()
	p := m.procs[id]
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.status == StatusStopped || p.status == StatusStopping {
		p.mu.Unlock()
		return nil
	}
	p.status = StatusStopping
	p.mu.Unlock()

	if p.cmd != nil && p.cmd.Process != nil {
		syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		if p.cancel != nil {
			p.cancel()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-p.done
	}
	return nil
}

func (m *Manager) Restart(id int64) error {
	if err := m.Stop(id); err != nil {
		return err
	}
	in, err := m.store.GetInstance(id)
	if err != nil {
		return err
	}
	return m.Start(in)
}

// RestartIfRunning 配置变更后调用：实例在跑则重启生效
func (m *Manager) RestartIfRunning(id int64) error {
	if !m.IsRunning(id) {
		return nil
	}
	return m.Restart(id)
}

func (m *Manager) StartEnabled() {
	list, err := m.store.ListEnabledInstances()
	if err != nil {
		return
	}
	for _, in := range list {
		if err := m.Start(in); err != nil {
			m.mu.Lock()
			p := &proc{status: StatusError, lastError: err.Error()}
			m.procs[in.ID] = p
			m.mu.Unlock()
		}
		time.Sleep(300 * time.Millisecond)
	}
	m.ReconcilePinnedVersions()
}

// ClearPinnedDownloadFailure 清除某版本的补下载失败标记，供用户重新保存服务器时重试
func (m *Manager) ClearPinnedDownloadFailure(version string) {
	if version == "" {
		return
	}
	m.mu.Lock()
	delete(m.downloadFailed, version)
	m.mu.Unlock()
}

// ReconcilePinnedVersions 为启用且指定版本（pin）尚未下载的实例补下载并自动切换：
// 同版本正在下载则跳过；下载成功后重启 pin 该版本的运行中实例（隧道中断数秒自恢复，
// 与重启语义一致）；失败（含版本已从远程消失）不重试、不报错给前端，保持默认版本，
// 下次进程启动或保存服务器时再 reconcile。
func (m *Manager) ReconcilePinnedVersions() {
	list, err := m.store.ListEnabledInstances()
	if err != nil {
		return
	}
	for _, in := range list {
		pin := in.FrpcVersion
		if pin == "" || m.HasVersion(pin) {
			continue
		}
		m.mu.Lock()
		_, failed := m.downloadFailed[pin]
		m.mu.Unlock()
		if failed {
			continue
		}
		if st, _ := m.DownloadStatus(pin); st == "downloading" {
			continue
		}
		if err := m.downloadFunc(pin, "github"); err != nil {
			m.mu.Lock()
			m.downloadFailed[pin] = struct{}{}
			m.mu.Unlock()
			continue
		}
		go m.watchPinnedDownload(pin)
	}
}

// watchPinnedDownload 轮询补下载任务至终态；不碰真实下载逻辑，便于单测注入
func (m *Manager) watchPinnedDownload(version string) {
	poll := m.watchPoll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	for {
		status, _ := m.DownloadStatus(version)
		switch status {
		case "downloading":
			time.Sleep(poll)
		case "done":
			m.mu.Lock()
			delete(m.downloadFailed, version)
			m.mu.Unlock()
			m.restartPinnedInstances(version)
			return
		default: // error / none：失败或任务丢失，保持默认版本不处理
			m.mu.Lock()
			m.downloadFailed[version] = struct{}{}
			m.mu.Unlock()
			return
		}
	}
}

func (m *Manager) restartPinnedInstances(version string) {
	list, err := m.store.ListEnabledInstances()
	if err != nil {
		return
	}
	for _, in := range list {
		if in.FrpcVersion == version && m.IsRunning(in.ID) {
			_ = m.restartFunc(in.ID)
		}
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// ProxyStatus 通过 frpc Admin API 查询代理在 frps 侧的状态
