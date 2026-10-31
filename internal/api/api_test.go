package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"frpc-mesh/internal/model"
	"frpc-mesh/internal/runner"
	"frpc-mesh/internal/store"
)

// 创建代理接口用外层 *bool 承接 enabled（遮蔽 model.Proxy.Enabled），
// 校验两种请求体都能落到正确的启用状态
func TestCreateProxyPayloadEnabled(t *testing.T) {
	resolve := func(body string) bool {
		var payload struct {
			model.Proxy
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		p := payload.Proxy
		if payload.Enabled != nil {
			p.Enabled = *payload.Enabled
		} else {
			p.Enabled = true
		}
		return p.Enabled
	}
	if !resolve(`{"name":"ssh","type":"tcp","enabled":true}`) {
		t.Error("显式 enabled=true 应保留为启用")
	}
	if resolve(`{"name":"ssh","type":"tcp","enabled":false}`) {
		t.Error("显式 enabled=false 应保留为禁用")
	}
	if !resolve(`{"name":"ssh","type":"tcp"}`) {
		t.Error("未带 enabled 字段应默认启用")
	}
}

// 设置接口逐字段合并：请求体未带的开关必须保持原值，
// 否则旧客户端只提交 auto_start 时会顺手关掉后来新增的 auto_update_check
func TestMergeSettingsPayload(t *testing.T) {
	merge := func(body string, cur model.Settings) model.Settings {
		var p settingsPayload
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		return mergeSettings(cur, p)
	}
	base := model.Settings{AutoStart: true, AutoUpdateCheck: true}

	if got := merge(`{"auto_start":false}`, base); !got.AutoUpdateCheck || got.AutoStart {
		t.Errorf("未提交 auto_update_check 应保持: %+v", got)
	}
	if got := merge(`{"auto_start":true,"auto_update_check":false}`, base); got.AutoUpdateCheck {
		t.Error("显式 false 应关闭自动检查")
	}
	if got := merge(`{"auto_update_check":true}`, model.Settings{}); !got.AutoUpdateCheck || got.AutoStart {
		t.Errorf("只提交 auto_update_check 时不应改动 auto_start: %+v", got)
	}
}

func TestIsPrivateIP(t *testing.T) {
	private := []string{"127.0.0.1", "10.0.0.5", "172.16.0.1", "192.168.1.50", "169.254.0.1", "::1", "fd00::1", "172.17.0.1", "100.64.0.1", "100.127.255.255", "::ffff:192.168.1.50"}
	public := []string{"8.8.8.8", "47.116.164.33", "203.0.113.9", "", "not-an-ip", "127.0.0.1|admin", "100.128.0.1", "100.63.255.255"}
	for _, s := range private {
		if !isPrivateIP(s) {
			t.Errorf("%s 应判为内网", s)
		}
	}
	for _, s := range public {
		if isPrivateIP(s) {
			t.Errorf("%s 应判为非内网", s)
		}
	}
}

// ---------- 测试脚手架：直挂路由的 Server（跳过 auth），不碰真实网络 ----------

func newTestServer(t *testing.T) (*Server, *store.Store, *runner.Manager, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "mesh.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	mgr := runner.NewManager(st, t.TempDir())
	s := &Server{Store: st, Mgr: mgr}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/proxies/check-port", s.handleCheckPort)
	mux.HandleFunc("POST /api/instances/{id}/proxies", s.handleCreateProxy)
	mux.HandleFunc("PUT /api/proxies/{pid}", s.handleUpdateProxy)
	t.Cleanup(func() {
		mgr.StopAll()
		st.Close()
	})
	return s, st, mgr, mux
}

func mustInstance(t *testing.T, st *store.Store, name, addr string, port int, pin string) *model.Instance {
	t.Helper()
	in := &model.Instance{Name: name, ServerAddr: addr, ServerPort: port,
		AuthMethod: "token", FrpcVersion: pin}
	if err := st.CreateInstance(in); err != nil {
		t.Fatalf("创建实例失败: %v", err)
	}
	return in
}

func mustProxy(t *testing.T, st *store.Store, instanceID int64, name, typ string, remotePort int, enabled bool) *model.Proxy {
	t.Helper()
	p := &model.Proxy{InstanceID: instanceID, Kind: "proxy", Name: name, Type: typ,
		LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: remotePort, Enabled: enabled}
	if err := st.CreateProxy(p); err != nil {
		t.Fatalf("创建代理失败: %v", err)
	}
	return p
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// checkPort 调检测接口并把响应按字符串字段解出（state/holder/note 均为字符串或空）
func checkPort(t *testing.T, h http.Handler, body string) (int, map[string]string) {
	t.Helper()
	rec := doJSON(t, h, "POST", "/api/proxies/check-port", body)
	var m map[string]string
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("解析 check-port 响应失败: %v body=%s", err, rec.Body.String())
		}
	}
	return rec.Code, m
}

func writeSleepFrpc(t *testing.T, mgr *runner.Manager, version string) {
	t.Helper()
	p := filepath.Join(mgr.VersionsDir(), "frpc-"+version)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("写入假 frpc 失败: %v", err)
	}
}

// ---------- 任务 4：check-port ----------

func TestCheckPortParamValidation(t *testing.T) {
	_, st, _, h := newTestServer(t)
	if code, _ := checkPort(t, h, `{"instance_id":999,"type":"tcp","remote_port":8080}`); code != 400 {
		t.Errorf("实例不存在应 400，实际 %d", code)
	}
	in := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	body := fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":0}`, in.ID)
	if code, _ := checkPort(t, h, body); code != 400 {
		t.Errorf("remote_port=0 应 400，实际 %d", code)
	}
	body = fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":-5}`, in.ID)
	if code, _ := checkPort(t, h, body); code != 400 {
		t.Errorf("remote_port=-5 应 400，实际 %d", code)
	}
}

func TestCheckPortOccupiedByOtherProxy(t *testing.T) {
	s, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "服务器A", "127.0.0.9", 7000, "")
	b := mustInstance(t, st, "服务器B", "127.0.0.9", 7000, "") // 同一 FRPS 服务器
	c := mustInstance(t, st, "服务器C", "127.0.0.8", 7000, "") // 另一台 FRPS
	mustProxy(t, st, b.ID, "备份口", "tcp", 8080, true)
	mustProxy(t, st, c.ID, "别的FRPS", "tcp", 8080, true)
	mustProxy(t, st, a.ID, "禁用", "tcp", 8080, false) // 禁用不算占用
	s.probeTCP = func(string) error {
		t.Error("本机查重命中时不应发起网络探测")
		return nil
	}

	code, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if code != 200 || m["state"] != "occupied" {
		t.Fatalf("跨服务器查重应命中 occupied，实际 code=%d %v", code, m)
	}
	if !strings.Contains(m["holder"], "服务器「服务器B」") || !strings.Contains(m["holder"], "代理「备份口」") {
		t.Errorf("holder 应写明服务器与代理名，实际 %q", m["holder"])
	}
	// 禁用代理不算占用：换 8081 只有禁用行不命中，注入拒绝探测 → free
	mustProxy(t, st, a.ID, "禁用2", "tcp", 8081, false)
	s.probeTCP = func(string) error { return syscall.ECONNREFUSED }
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8081}`, a.ID))
	if m["state"] != "free" {
		t.Errorf("禁用代理不算冲突，实际 %v", m)
	}
}

func TestCheckPortTCPProbeStates(t *testing.T) {
	s, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	req := fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID)

	// 拒绝 → free，且探测地址为 server_addr:remote_port
	s.probeTCP = func(addr string) error {
		if want := "127.0.0.9:8080"; addr != want {
			t.Errorf("探测地址应为 %q，实际 %q", want, addr)
		}
		return syscall.ECONNREFUSED
	}
	code, m := checkPort(t, h, req)
	if code != 200 || m["state"] != "free" {
		t.Errorf("连接被拒绝应 free，实际 code=%d %v", code, m)
	}

	// 连上且本机无持有者 → occupied（服务端已有监听）
	s.probeTCP = func(string) error { return nil }
	_, m = checkPort(t, h, req)
	if m["state"] != "occupied" || m["holder"] != "服务端已有监听（非本机配置）" {
		t.Errorf("探测连上应 occupied 非本机配置，实际 %v", m)
	}

	// 超时/其它错误 → unknown + note，不阻断保存
	s.probeTCP = func(string) error { return os.ErrDeadlineExceeded }
	_, m = checkPort(t, h, req)
	if m["state"] != "unknown" || !strings.Contains(m["note"], "未阻断保存") {
		t.Errorf("探测超时应 unknown 并说明不阻断，实际 %v", m)
	}
}

func TestCheckPortSelfExclusion(t *testing.T) {
	s, st, mgr, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "9.9.9")
	writeSleepFrpc(t, mgr, "9.9.9")
	px := mustProxy(t, st, a.ID, "自身", "tcp", 8080, true)
	s.probeTCP = func(string) error { return nil } // 总能连上

	// 不排除自身 → 查重直接 occupied
	req := fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID)
	_, m := checkPort(t, h, req)
	if m["state"] != "occupied" {
		t.Fatalf("未排除自身应 occupied，实际 %v", m)
	}

	// 排除自身且其实例正在运行（占用可解释）→ free
	if err := mgr.Start(a); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	excl := fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080,"exclude_proxy_id":%d}`, a.ID, px.ID)
	_, m = checkPort(t, h, excl)
	if m["state"] != "free" {
		t.Errorf("排除运行中的自身应 free，实际 %v", m)
	}

	// 实例停止后再“连上”说明不是本机配置 → occupied
	if err := mgr.Stop(a.ID); err != nil {
		t.Fatal(err)
	}
	_, m = checkPort(t, h, excl)
	if m["state"] != "occupied" || m["holder"] != "服务端已有监听（非本机配置）" {
		t.Errorf("自身未运行时连上应 occupied 非本机配置，实际 %v", m)
	}
}

func TestCheckPortUDP(t *testing.T) {
	s, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	mustProxy(t, st, a.ID, "dns", "udp", 53, true)
	probed := false
	s.probeTCP = func(string) error { probed = true; return nil }

	// udp 查重命中 → occupied
	_, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"udp","remote_port":53}`, a.ID))
	if m["state"] != "occupied" || !strings.Contains(m["holder"], "dns") {
		t.Fatalf("udp 查重命中应 occupied，实际 %v", m)
	}
	// udp 未命中 → unknown 并说明限制，不发起探测
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"udp","remote_port":5353}`, a.ID))
	if m["state"] != "unknown" || !strings.Contains(m["note"], "udp") {
		t.Errorf("udp 未命中应 unknown 并说明限制，实际 %v", m)
	}
	// tcp 请求不受 udp 代理影响（协议不同不互斥）
	s.probeTCP = func(string) error { return syscall.ECONNREFUSED }
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":53}`, a.ID))
	if m["state"] != "free" {
		t.Errorf("tcp/udp 不应互相判占用，实际 %v", m)
	}
	if probed {
		t.Error("udp 请求不应发起 TCP 探测")
	}
}
