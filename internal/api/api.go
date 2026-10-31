package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"frpc-mesh/internal/auth"
	"frpc-mesh/internal/model"
	"frpc-mesh/internal/netinfo"
	"frpc-mesh/internal/runner"
	"frpc-mesh/internal/store"
	"frpc-mesh/internal/update"
)

type Server struct {
	Store   *store.Store
	Mgr     *runner.Manager
	Auth    *auth.Service
	Login   *auth.Limiter
	Update  *update.Checker
	Upgrade *update.Upgrader
	Version string
	WebFS   fs.FS // 前端静态资源根（已含 index.html）
	// probeTCP 端口探测注入点，默认 net.DialTimeout；单测替换以隔离真实网络
	probeTCP func(addr string) error
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

var validProxyTypes = map[string]bool{
	"tcp": true, "udp": true, "http": true, "https": true,
	"stcp": true, "sudp": true, "xtcp": true,
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": s.Version})
	})
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/auth/me", s.handleMe)
	protected.HandleFunc("PUT /api/auth/password", s.handleChangePassword)
	protected.HandleFunc("GET /api/dashboard", s.handleDashboard)
	protected.HandleFunc("GET /api/instances", s.handleListInstances)
	protected.HandleFunc("POST /api/instances", s.handleCreateInstance)
	protected.HandleFunc("GET /api/instances/{id}", s.handleGetInstance)
	protected.HandleFunc("PUT /api/instances/{id}", s.handleUpdateInstance)
	protected.HandleFunc("DELETE /api/instances/{id}", s.handleDeleteInstance)
	protected.HandleFunc("POST /api/instances/{id}/start", s.handleStart)
	protected.HandleFunc("POST /api/instances/{id}/stop", s.handleStop)
	protected.HandleFunc("POST /api/instances/{id}/restart", s.handleRestart)
	protected.HandleFunc("GET /api/instances/{id}/logs", s.handleLogs)
	protected.HandleFunc("POST /api/instances/{id}/logs/clear", s.handleLogsClear)
	protected.HandleFunc("POST /api/instances/{id}/proxies/ping", s.handlePingProxies)
	protected.HandleFunc("GET /api/instances/{id}/config", s.handleConfig)
	protected.HandleFunc("GET /api/instances/{id}/proxies/online", s.handleProxiesOnline)
	protected.HandleFunc("GET /api/instances/{id}/proxies", s.handleListProxies)
	protected.HandleFunc("POST /api/instances/{id}/proxies", s.handleCreateProxy)
	protected.HandleFunc("POST /api/proxies/check-port", s.handleCheckPort)
	protected.HandleFunc("PUT /api/proxies/{pid}", s.handleUpdateProxy)
	protected.HandleFunc("PUT /api/proxies/{pid}/enabled", s.handleSetProxyEnabled)
	protected.HandleFunc("DELETE /api/proxies/{pid}", s.handleDeleteProxy)
	protected.HandleFunc("GET /api/settings", s.handleGetSettings)
	protected.HandleFunc("PUT /api/settings", s.handlePutSettings)
	protected.HandleFunc("GET /api/settings/net-hints", s.handleNetHints)
	protected.HandleFunc("GET /api/settings/backup", s.handleExportBackup)
	protected.HandleFunc("POST /api/settings/backup/restore", s.handleRestoreBackup)
	protected.HandleFunc("GET /api/frpc/versions", s.handleListVersions)
	protected.HandleFunc("GET /api/frpc/versions/local", s.handleListVersionsLocal)
	protected.HandleFunc("POST /api/frpc/versions/download", s.handleDownloadVersion)
	protected.HandleFunc("GET /api/frpc/versions/download/status", s.handleDownloadStatus)
	protected.HandleFunc("POST /api/frpc/versions/activate", s.handleActivateVersion)
	protected.HandleFunc("DELETE /api/frpc/versions/{version}", s.handleDeleteVersion)
	protected.HandleFunc("GET /api/update/status", s.handleUpdateStatus)
	protected.HandleFunc("POST /api/update/check", s.handleUpdateCheck)
	protected.HandleFunc("POST /api/update/upgrade", s.handleUpgradeStart)
	protected.HandleFunc("GET /api/update/upgrade/status", s.handleUpgradeStatus)
	mux.Handle("/api/", s.Auth.Middleware(protected))

	mux.HandleFunc("/", s.spaHandler())
	return mux
}

// ---------- auth ----------

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isPrivateIP 判断内网来源：环回/RFC1918/链路本地/IPv6 ULA/CGNAT(100.64/10，Tailscale 等组网)
// 只看连接来源 IP，与用户用哪个域名访问无关（域名最终都解析成这些 IP 之一）
func isPrivateIP(s string) bool {
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// CGNAT：Tailscale/Flynet 等 VPN 组网地址段
	return ip.Is4() && ip.Compare(netip.AddrFrom4([4]byte{100, 64, 0, 0})) >= 0 &&
		ip.Compare(netip.AddrFrom4([4]byte{100, 127, 255, 255})) <= 0
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	ip := clientIP(r)
	key := ip + "|" + strings.TrimSpace(req.Username)
	if left, locked := s.Login.LockedFor(key, time.Now()); locked {
		msg := fmt.Sprintf("失败次数过多，已锁定，请 %d 秒后再试", int(left.Seconds()))
		if isPrivateIP(ip) {
			msg += "；若确认忘记密码，可在宿主机执行 docker exec -it frpc-mesh /app/frpc-mesh --reset-password --data /data 重置为 admin123（详见 docs/operations.md）"
		}
		writeErr(w, 429, msg)
		return
	}
	hash, err := s.Store.GetUserHash(strings.TrimSpace(req.Username))
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		s.Login.Fail(key, time.Now())
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	s.Login.Success(key)
	tok, err := s.Auth.IssueToken(req.Username)
	if err != nil {
		writeErr(w, 500, "生成令牌失败")
		return
	}
	writeJSON(w, 200, map[string]string{"token": tok})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"username": "admin"})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil || len(req.NewPassword) < 6 {
		writeErr(w, 400, "新密码至少 6 位")
		return
	}
	hash, err := s.Store.GetUserHash("admin")
	if err != nil || !auth.CheckPassword(hash, req.OldPassword) {
		writeErr(w, 400, "旧密码错误")
		return
	}
	nh, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	if err := s.Store.UpsertUser("admin", nh); err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- dashboard ----------

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	instances, err := s.Store.ListInstances()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	online := 0
	proxiesOnline := 0
	type brief struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Status        string `json:"status"`
		Enabled       bool   `json:"enabled"`
		ServerAddr    string `json:"server_addr"`
		ServerPort    int    `json:"server_port"`
		ProxiesTotal  int    `json:"proxies_total"`
		ProxiesOnline int    `json:"proxies_online"`
	}
	briefs := []brief{}
	for _, in := range instances {
		rt := s.Mgr.Runtime(in.ID)
		b := brief{ID: in.ID, Name: in.Name, Status: rt.Status, Enabled: in.Enabled,
			ServerAddr: in.ServerAddr, ServerPort: in.ServerPort,
			ProxiesTotal: s.Store.CountProxiesByInstance(in.ID)}
		if rt.Status == runner.StatusOnline {
			online++
			for _, p := range s.usableProxies(in) {
				if p.Kind == "proxy" {
					b.ProxiesOnline++
				}
			}
		}
		proxiesOnline += b.ProxiesOnline
		briefs = append(briefs, b)
	}
	writeJSON(w, 200, map[string]any{
		"instances_total":  len(instances),
		"instances_online": online,
		"proxies_total":    s.Store.CountActiveProxies("proxy"),
		"proxies_online":   proxiesOnline,
		"visitors_total":   s.Store.CountActiveProxies("visitor"),
		"instances":        briefs,
	})
}

// usableProxies 在线代理 = frpc 日志确认隧道建立 + 本地目标可连通；
// visitor 与 udp 无法/无需拨测本地目标，仅以隧道建立为准
func (s *Server) usableProxies(in *model.Instance) []*model.Proxy {
	running := s.Mgr.ProxyRunningNames(in.ID)
	if len(running) == 0 {
		return nil
	}
	proxies, err := s.Store.ListProxies(in.ID)
	if err != nil {
		return nil
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	out := []*model.Proxy{}
	for _, px := range proxies {
		if !running[px.Name] {
			continue
		}
		if px.Kind == "visitor" || px.Type == "udp" {
			mu.Lock()
			out = append(out, px)
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(p *model.Proxy) {
			defer wg.Done()
			if !localTargetReachable(p.LocalIP, p.LocalPort) {
				return
			}
			mu.Lock()
			out = append(out, p)
			mu.Unlock()
		}(px)
	}
	wg.Wait()
	return out
}

func (s *Server) handleProxiesOnline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	in, err := s.Store.GetInstance(id)
	if err != nil {
		writeErr(w, 404, "服务器不存在")
		return
	}
	ids := []int64{}
	for _, p := range s.usableProxies(in) {
		ids = append(ids, p.ID)
	}
	writeJSON(w, 200, map[string][]int64{"online_ids": ids})
}

// localTargetReachable 在数据层判定本地目标是否存活：容器内 vpnkit 会"预接受"任何出站连接，
// 连接建立不代表端口活着；宿主机侧端口为立即拨号，死端口会马上 EOF/RST，据此区分
func localTargetReachable(ip string, port int) bool {
	if port <= 0 {
		return false
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), 1500*time.Millisecond)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	_, err = c.Read(make([]byte, 1))
	if err == nil {
		return true // 服务主动发来数据
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true // 连接存活，服务静默
	}
	return false // EOF/RST：对端立即关闭，端口死的
}

// ---------- instances ----------

func (s *Server) withRuntime(in *model.Instance) *model.Instance {
	in.Runtime = s.Mgr.Runtime(in.ID)
	return in
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListInstances()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]*model.Instance, 0, len(list))
	for _, in := range list {
		out = append(out, s.withRuntime(in))
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	in, err := s.Store.GetInstance(id)
	if err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	writeJSON(w, 200, s.withRuntime(in))
}

func (s *Server) validateInstance(in *model.Instance) string {
	if strings.TrimSpace(in.Name) == "" {
		return "实例名称不能为空"
	}
	if strings.TrimSpace(in.ServerAddr) == "" {
		return "服务器地址不能为空"
	}
	if in.ServerPort <= 0 || in.ServerPort > 65535 {
		return "服务器端口需在 1-65535 之间"
	}
	if in.FrpcVersion != "" && !runner.ValidVersion(in.FrpcVersion) {
		return "frpc 版本需为 x.y.z 形式"
	}
	if in.AdminCfg.Enabled {
		if in.AdminCfg.Port <= 0 || in.AdminCfg.Port > 65535 {
			return "Admin API 端口需在 1-65535 之间"
		}
		for _, p := range s.Store.UsedAdminPorts(in.ID) {
			if p == in.AdminCfg.Port {
				return fmt.Sprintf("Admin API 端口 %d 已被其他实例占用", p)
			}
		}
	}
	return ""
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var in model.Instance
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	in.ID = 0
	if in.AuthMethod == "" {
		in.AuthMethod = "token"
	}
	if in.ServerPort == 0 {
		in.ServerPort = 7000
	}
	if msg := s.validateInstance(&in); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := s.Store.CreateInstance(&in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 指定的 frpc 版本可能还没下载，异步补下载并在成功后自动切换
	s.Mgr.ClearPinnedDownloadFailure(in.FrpcVersion)
	go s.Mgr.ReconcilePinnedVersions()
	writeJSON(w, 201, s.withRuntime(&in))
}

func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	old, err := s.Store.GetInstance(id)
	if err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	var in model.Instance
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	in.ID = id
	in.CreatedAt = old.CreatedAt
	if msg := s.validateInstance(&in); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := s.Store.UpdateInstance(&in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 启用=立即启动+自启；停用=立即停止+不自启；其余字段变更则在运行中时重启生效
	if in.Enabled != old.Enabled {
		if in.Enabled {
			s.Mgr.Start(&in)
		} else {
			s.Mgr.Stop(id)
		}
	} else {
		s.Mgr.RestartIfRunning(id)
	}
	s.Mgr.ClearPinnedDownloadFailure(in.FrpcVersion)
	go s.Mgr.ReconcilePinnedVersions()
	writeJSON(w, 200, s.withRuntime(&in))
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	s.Mgr.Stop(id)
	if err := s.Store.DeleteInstance(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) instanceAction(w http.ResponseWriter, r *http.Request, fn func(*model.Instance) error) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	in, err := s.Store.GetInstance(id)
	if err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	if err := fn(in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	s.instanceAction(w, r, s.Mgr.Start)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.instanceAction(w, r, func(in *model.Instance) error { return s.Mgr.Stop(in.ID) })
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	s.instanceAction(w, r, func(in *model.Instance) error {
		s.Mgr.Stop(in.ID)
		return s.Mgr.Start(in)
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 500
	}
	writeJSON(w, 200, map[string][]string{"lines": s.Mgr.Logs(id, tail)})
}

func (s *Server) handleLogsClear(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	s.Mgr.ClearLogs(id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type pingResult struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	LatencyMs int64  `json:"latency_ms"` // -1 不通/不可测；0 可通（服务静默）；>0 往返延迟毫秒
}

// handlePingProxies 从本机向 frps 的远程端口发起 TCP 连接，测量隧道延迟
func (s *Server) handlePingProxies(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	in, err := s.Store.GetInstance(id)
	if err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	proxies, err := s.Store.ListProxies(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]pingResult, len(proxies))
	var wg sync.WaitGroup
	for i, p := range proxies {
		out[i] = pingResult{ID: p.ID, Name: p.Name, Type: p.Type, LatencyMs: -1}
		if !(p.Kind == "proxy" && p.Enabled && p.Type == "tcp" && p.RemotePort > 0) {
			continue
		}
		wg.Add(1)
		go func(idx int, p *model.Proxy) {
			defer wg.Done()
			addr := fmt.Sprintf("%s:%d", in.ServerAddr, p.RemotePort)
			start := time.Now()
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()
			// 容器内 vpnkit 使连接建立恒为 0ms，需写探针触发宿主机侧真实拨号，再以数据层响应判定：
			// 收到字节 → 延迟=写入到首字节耗时；探针写出后静默超时 → 隧道通(0)；EOF/RST/写失败 → 不通(-1)
			_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
			_, werr := conn.Write([]byte("\r\n"))
			// 安全组丢弃型死端口：vpnkit 宿主机侧拨号约 5s 才失败，读窗口需覆盖
			_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
			_, rerr := conn.Read(make([]byte, 1))
			elapsed := time.Since(start).Milliseconds()
			var ne net.Error
			switch {
			case rerr == nil:
				out[idx].LatencyMs = elapsed
			case werr == nil && errors.As(rerr, &ne) && ne.Timeout():
				out[idx].LatencyMs = 0
			}
		}(i, p)
	}
	wg.Wait()
	writeJSON(w, 200, out)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	toml, err := s.Mgr.PreviewConfig(id)
	if err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	writeJSON(w, 200, map[string]string{"toml": toml})
}

// ---------- proxies ----------

func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	list, err := s.Store.ListProxies(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]*model.Proxy, 0, len(list))
	out = append(out, list...)
	writeJSON(w, 200, out)
}

func (s *Server) validateProxy(p *model.Proxy) string {
	if strings.TrimSpace(p.Name) == "" {
		return "代理名称不能为空"
	}
	if !validProxyTypes[p.Type] {
		return "不支持的代理类型: " + p.Type
	}
	if p.Kind != "proxy" && p.Kind != "visitor" {
		return "kind 必须是 proxy 或 visitor"
	}
	if p.Kind == "visitor" {
		if p.Type != "stcp" && p.Type != "xtcp" && p.Type != "sudp" {
			return "访问器类型必须是 stcp/xtcp/sudp"
		}
		if p.BindPort <= 0 || p.BindPort > 65535 {
			return "访问器 bind_port 需在 1-65535 之间"
		}
		if strings.TrimSpace(p.ServerName) == "" {
			return "访问器需要填写 server_name"
		}
	}
	return ""
}

// remotePortConflict 保存兜底查重（设计 §3.2）：与 check-port 的本机口径一致——
// 同 FRPS 服务器（addr+port 相同）、启用、同协议 tcp/udp、同远程端口、排除自身；不做网络探测
func (s *Server) remotePortConflict(p *model.Proxy) string {
	// 禁用的代理不写入 frpc 配置、也不占用服务端端口，不该被拦
	if (p.Type != "tcp" && p.Type != "udp") || p.RemotePort <= 0 || !p.Enabled {
		return ""
	}
	in, err := s.Store.GetInstance(p.InstanceID)
	if err != nil {
		return ""
	}
	holders, err := s.findRemotePortHolders(in, p.Type, p.RemotePort)
	if err != nil {
		return ""
	}
	for _, h := range holders {
		if h.ProxyID != p.ID {
			return fmt.Sprintf("远程端口 %d 已被代理「%s」占用", p.RemotePort, h.ProxyName)
		}
	}
	return ""
}

func (s *Server) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, "无效的实例 ID")
		return
	}
	if _, err := s.Store.GetInstance(id); err != nil {
		writeErr(w, 404, "实例不存在")
		return
	}
	var payload struct {
		model.Proxy
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	p := payload.Proxy
	p.ID = 0
	p.InstanceID = id
	if payload.Enabled != nil {
		p.Enabled = *payload.Enabled
	} else {
		p.Enabled = true // 旧客户端/旧备份未带该字段时默认启用
	}
	if p.Kind == "" {
		p.Kind = "proxy"
	}
	if msg := s.validateProxy(&p); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if msg := s.remotePortConflict(&p); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := s.Store.CreateProxy(&p); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Mgr.RestartIfRunning(id)
	writeJSON(w, 201, &p)
}

func (s *Server) handleUpdateProxy(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeErr(w, 400, "无效的代理 ID")
		return
	}
	old, err := s.Store.GetProxy(pid)
	if err != nil {
		writeErr(w, 404, "代理不存在")
		return
	}
	var p model.Proxy
	if err := readJSON(r, &p); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	p.ID = pid
	p.InstanceID = old.InstanceID
	if msg := s.validateProxy(&p); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if msg := s.remotePortConflict(&p); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := s.Store.UpdateProxy(&p); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Mgr.RestartIfRunning(p.InstanceID)
	writeJSON(w, 200, &p)
}

// handleSetProxyEnabled 启用/禁用代理：禁用的代理不写入 frpc 配置，重启生效
func (s *Server) handleSetProxyEnabled(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeErr(w, 400, "无效的代理 ID")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	old, err := s.Store.GetProxy(pid)
	if err != nil {
		writeErr(w, 404, "代理不存在")
		return
	}
	if body.Enabled {
		// 启用会把代理推给 frps，端口冲突要在这里拦住，否则隧道必然起不来
		old.Enabled = true
		if msg := s.remotePortConflict(old); msg != "" {
			writeErr(w, 400, msg)
			return
		}
	}
	if err := s.Store.SetProxyEnabled(pid, body.Enabled); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Mgr.RestartIfRunning(old.InstanceID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeErr(w, 400, "无效的代理 ID")
		return
	}
	old, err := s.Store.GetProxy(pid)
	if err != nil {
		writeErr(w, 404, "代理不存在")
		return
	}
	if err := s.Store.DeleteProxy(pid); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Mgr.RestartIfRunning(old.InstanceID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 远程端口占用检测 ----------

type checkPortRequest struct {
	InstanceID     int64  `json:"instance_id"`
	Type           string `json:"type"`
	RemotePort     int    `json:"remote_port"`
	ExcludeProxyID int64  `json:"exclude_proxy_id"`
}

type checkPortResponse struct {
	State  string `json:"state"`  // free | occupied | unknown；unknown 永不阻断保存
	Holder string `json:"holder"` // occupied 时的占用者描述
	Note   string `json:"note"`   // unknown 时的限制说明
}

type portHolder struct {
	ProxyID      int64
	InstanceID   int64
	InstanceName string
	ProxyName    string
}

// findRemotePortHolders 本机跨服务器查重：与目标实例同 server_addr+server_port 的实例下，
// 启用中、type 相同（tcp/udp 端口空间互不冲突）且远程端口相同的代理
func (s *Server) findRemotePortHolders(in *model.Instance, typ string, remotePort int) ([]portHolder, error) {
	instances, err := s.Store.ListInstances()
	if err != nil {
		return nil, err
	}
	var out []portHolder
	for _, peer := range instances {
		if peer.ServerAddr != in.ServerAddr || peer.ServerPort != in.ServerPort {
			continue
		}
		proxies, err := s.Store.ListProxies(peer.ID)
		if err != nil {
			return nil, err
		}
		for _, px := range proxies {
			if px.Enabled && px.Type == typ && px.RemotePort == remotePort {
				out = append(out, portHolder{px.ID, peer.ID, peer.Name, px.Name})
			}
		}
	}
	return out, nil
}

func (s *Server) dialRemotePort(addr string) error {
	if s.probeTCP != nil {
		return s.probeTCP(addr)
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}

// handleCheckPort 检测远程端口占用，口径见设计 §3.1：先本机跨服务器查重，tcp 再探测服务端
func (s *Server) handleCheckPort(w http.ResponseWriter, r *http.Request) {
	var req checkPortRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	in, err := s.Store.GetInstance(req.InstanceID)
	if err != nil {
		writeErr(w, 400, "实例不存在")
		return
	}
	if req.RemotePort <= 0 || req.RemotePort > 65535 {
		writeErr(w, 400, "远程端口需在 1-65535 之间")
		return
	}
	if req.Type != "tcp" && req.Type != "udp" {
		writeJSON(w, 200, checkPortResponse{State: "unknown", Note: "仅支持 tcp/udp 的远程端口检测"})
		return
	}
	holders, err := s.findRemotePortHolders(in, req.Type, req.RemotePort)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 1. 本机查重（排除当前编辑中的代理）
	for _, h := range holders {
		if h.ProxyID != req.ExcludeProxyID {
			writeJSON(w, 200, checkPortResponse{State: "occupied",
				Holder: fmt.Sprintf("服务器「%s」的代理「%s」", h.InstanceName, h.ProxyName)})
			return
		}
	}
	// 2. udp 无法从本机探测，查重未命中即明示限制
	if req.Type == "udp" {
		writeJSON(w, 200, checkPortResponse{State: "unknown",
			Note: "udp 端口无法从本机探测，仅确认本机其它代理未占用"})
		return
	}
	// 3. tcp 探测服务端
	err = s.dialRemotePort(net.JoinHostPort(in.ServerAddr, strconv.Itoa(req.RemotePort)))
	if err != nil && !errors.Is(err, syscall.ECONNREFUSED) {
		// 超时/DNS 失败等一律 unknown，不阻断保存（可能被安全组静默丢弃）
		writeJSON(w, 200, checkPortResponse{State: "unknown",
			Note: "连接超时：可能被安全组静默丢弃或服务不可达；未阻断保存"})
		return
	}
	if err != nil { // 明确拒绝 → 空闲
		writeJSON(w, 200, checkPortResponse{State: "free"})
		return
	}
	// 连得上：唯一持有者是“被排除的自身”且其实例在运行，才能解释为本机占用；否则视为服务端已有监听
	if len(holders) == 1 && holders[0].ProxyID == req.ExcludeProxyID && s.Mgr.IsRunning(holders[0].InstanceID) {
		writeJSON(w, 200, checkPortResponse{State: "free"})
		return
	}
	writeJSON(w, 200, checkPortResponse{State: "occupied", Holder: "服务端已有监听（非本机配置）"})
}

// ---------- settings ----------

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st := s.Store.GetSettings()
	st.Version = s.Version
	writeJSON(w, 200, st)
}

// settingsPayload 用 *bool 承接，请求未带的开关保持原值
type settingsPayload struct {
	AutoStart       *bool `json:"auto_start"`
	AutoUpdateCheck *bool `json:"auto_update_check"`
}

func mergeSettings(cur model.Settings, p settingsPayload) model.Settings {
	if p.AutoStart != nil {
		cur.AutoStart = *p.AutoStart
	}
	if p.AutoUpdateCheck != nil {
		cur.AutoUpdateCheck = *p.AutoUpdateCheck
	}
	return cur
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var payload settingsPayload
	if err := readJSON(r, &payload); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	st := mergeSettings(s.Store.GetSettings(), payload)
	if err := s.Store.SaveSettings(st); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	st.Version = s.Version
	writeJSON(w, 200, st)
}

// ---------- 新版本检测与一键升级 ----------

// updateResponse 汇总检测结果、环境可升级性与最近一次升级任务，供前端一次取全
func (s *Server) updateResponse(inf *update.Info) map[string]any {
	if inf.Assets == nil {
		inf.Assets = []update.Asset{}
	}
	return map[string]any{
		"info":         inf,
		"upgrade":      s.Upgrade.CapabilityFor(inf.Latest, inf.Assets),
		"upgrade_job":  s.Upgrade.Status(),
		"last_upgrade": s.Update.LastUpgrade(),
	}
}

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.updateResponse(s.Update.Status()))
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	source := "github"
	if r.URL.Query().Get("source") == "ghfast" {
		source = "ghfast"
	}
	writeJSON(w, 200, s.updateResponse(s.Update.Result(source)))
}

func (s *Server) handleUpgradeStart(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Version string `json:"version"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.Upgrade.Start(strings.TrimSpace(payload.Version)); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, s.Upgrade.Status())
}

func (s *Server) handleUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Upgrade.Status())
}

func (s *Server) handleNetHints(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, netinfo.Detect())
}

// handleExportBackup 导出全部服务器与代理配置为单个 JSON
func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	instances, err := s.Store.ListInstances()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := model.Backup{
		App:        "frpc-mesh",
		Version:    2,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Instances:  []model.BackupInstance{},
	}
	for _, in := range instances {
		in.Runtime = nil
		proxies, err := s.Store.ListProxies(in.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		bi := model.BackupInstance{Instance: *in, Proxies: proxies}
		if bi.Proxies == nil {
			bi.Proxies = []*model.Proxy{}
		}
		out.Instances = append(out.Instances, bi)
	}
	writeJSON(w, 200, out)
}

// handleRestoreBackup 以备份数据为准：停止全部进程后清空现有服务器/代理并重建，再拉起已启用的
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	var bk model.Backup
	if err := json.NewDecoder(r.Body).Decode(&bk); err != nil || bk.Instances == nil {
		writeErr(w, 400, "备份文件格式无效")
		return
	}
	s.Mgr.StopAll()
	old, err := s.Store.ListInstances()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, in := range old {
		if err := s.Store.DeleteInstance(in.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	proxyCount := 0
	for _, bi := range bk.Instances {
		in := bi.Instance
		in.ID = 0
		in.Runtime = nil
		if err := s.Store.CreateInstance(&in); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, p := range bi.Proxies {
			p.ID = 0
			p.InstanceID = in.ID
			if bk.Version < 2 {
				p.Enabled = true // v1 备份无 enabled 字段
			}
			if err := s.Store.CreateProxy(p); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			proxyCount++
		}
	}
	s.Mgr.StartEnabled()
	writeJSON(w, 200, map[string]int{"instances": len(bk.Instances), "proxies": proxyCount})
}

// ---------- frpc 版本管理 ----------

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	remote, err := s.Mgr.FetchRemoteVersionsCached(source)
	merged := s.Mgr.MergeVersions(remote)
	resp := map[string]any{
		"active":   s.Mgr.ActiveVersion(),
		"current":  s.Mgr.FrpcVersion(),
		"versions": merged,
		"local":    s.Mgr.LocalVersions(),
	}
	if resp["versions"] == nil {
		resp["versions"] = []any{}
	}
	if resp["local"] == nil {
		resp["local"] = []any{}
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

// handleListVersionsLocal 仅本地版本，秒回，供页面首屏立即渲染
func (s *Server) handleListVersionsLocal(w http.ResponseWriter, r *http.Request) {
	list := s.Mgr.LocalVersions()
	resp := map[string]any{
		"active":   s.Mgr.ActiveVersion(),
		"current":  s.Mgr.FrpcVersion(),
		"versions": list,
	}
	if resp["versions"] == nil {
		resp["versions"] = []any{}
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleDownloadVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
		Source  string `json:"source"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.Mgr.StartDownload(req.Version, req.Source); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleDownloadStatus(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	status, msg := s.Mgr.DownloadStatus(version)
	writeJSON(w, 200, map[string]string{"status": status, "message": msg})
}

func (s *Server) handleActivateVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.Mgr.ActivateVersion(req.Version); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	if err := s.Mgr.DeleteVersion(r.PathValue("version")); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- SPA ----------

func (s *Server) spaHandler() http.HandlerFunc {
	fileServer := http.FileServerFS(s.WebFS)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, 404, "接口不存在")
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := s.WebFS.Open(p); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	}
}
