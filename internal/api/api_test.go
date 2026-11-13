package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	public := []string{"8.8.8.8", "198.51.100.23", "203.0.113.9", "", "not-an-ip", "127.0.0.1|admin", "100.128.0.1", "100.63.255.255"}
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
	mux.HandleFunc("POST /api/instances", s.handleCreateInstance)
	mux.HandleFunc("PUT /api/proxies/{pid}/enabled", s.handleSetProxyEnabled)
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

// setDash 给实例配置 frps 管理接口：端口写进 manage_port，凭据写进 dashboard
func setDash(t *testing.T, st *store.Store, id int64, port int, user, pwd string) {
	t.Helper()
	in, err := st.GetInstance(id)
	if err != nil {
		t.Fatal(err)
	}
	in.ManagePort = port
	in.Dashboard = model.DashAuth{User: user, Password: pwd}
	if err := st.UpdateInstance(in); err != nil {
		t.Fatal(err)
	}
}

func dashPort(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestCheckPortOccupiedByOtherProxy(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "服务器A", "127.0.0.1", 7000, "")
	b := mustInstance(t, st, "服务器B", "127.0.0.1", 7000, "") // 同一 FRPS 服务器
	c := mustInstance(t, st, "服务器C", "127.0.0.8", 7000, "") // 另一台 FRPS
	mustProxy(t, st, b.ID, "备份口", "tcp", 8080, true)
	mustProxy(t, st, c.ID, "别的FRPS", "tcp", 8080, true)
	mustProxy(t, st, a.ID, "禁用", "tcp", 8080, false) // 禁用不算占用
	// 本机查重是硬结论，配了管理端口也不该再去问远端
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
		fmt.Fprint(w, `{"proxies":[]}`)
	}))
	defer srv.Close()
	setDash(t, st, a.ID, dashPort(t, srv.URL), "", "")

	code, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if code != 200 || m["state"] != "occupied" {
		t.Fatalf("跨服务器查重应命中 occupied，实际 code=%d %v", code, m)
	}
	if !strings.Contains(m["holder"], "服务器「服务器B」") || !strings.Contains(m["holder"], "代理「备份口」") {
		t.Errorf("holder 应写明服务器与代理名，实际 %q", m["holder"])
	}
	if asked {
		t.Error("本机查重命中时不该再请求 frps 管理接口")
	}
	// 禁用代理不算占用，且 frps 表里也没有 → free
	mustProxy(t, st, a.ID, "禁用2", "tcp", 8081, false)
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8081}`, a.ID))
	if m["state"] != "free" {
		t.Errorf("禁用行与 frps 空表应 free，实际 %v", m)
	}
}

// 没配 frps 管理端口时只能说「无法确定」：早先这里靠 TCP 能不能连上推断，
// 而容器/公司网络的透明代理会替任意端口完成握手，把空闲端口报成占用并拦住保存
func TestCheckPortWithoutDashIsUnknown(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	_, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if m["state"] != "unknown" || !strings.Contains(m["note"], "无法确定") || !strings.Contains(m["note"], "不阻断") {
		t.Errorf("未配置管理端口应 unknown 且说明不阻断，实际 %v", m)
	}
}

func TestCheckPortViaFrpsAPI(t *testing.T) {
	var gotAuth, gotPath string
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.1", 7000, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPwd, isBasic := r.BasicAuth()
		gotAuth = fmt.Sprintf("%s basic=%v %s", gotUser, isBasic, gotPwd)
		fmt.Fprint(w, `{"proxies":[{"name":"别人建的隧道","conf":{"remotePort":8080}},{"name":"随机端口","conf":{"remotePort":0}}]}`)
	}))
	defer srv.Close()
	setDash(t, st, a.ID, dashPort(t, srv.URL), "dash", "pw")

	_, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if m["state"] != "occupied" || !strings.Contains(m["holder"], "别人建的隧道") {
		t.Errorf("frps 表里有这条应 occupied 并写出代理名，实际 %v", m)
	}
	if gotPath != "/api/proxy/tcp" {
		t.Errorf("应查 /api/proxy/tcp，实际 %q", gotPath)
	}
	if gotAuth != "dash basic=true pw" {
		t.Errorf("应带 dashboard Basic 凭据，实际 %q", gotAuth)
	}
	// 表里没有 → free；remotePort=0（随机分配）不参与判定
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8081}`, a.ID))
	if m["state"] != "free" {
		t.Errorf("frps 表里没有应 free，实际 %v", m)
	}
	code, bad := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":0}`, a.ID))
	if code != 400 || bad["state"] != "" {
		t.Errorf("remote_port=0 属参数非法应 400，实际 code=%d %v", code, bad)
	}
}

func TestDashBaseBracketsIPv6(t *testing.T) {
	if got := dashBase(&model.Instance{ServerAddr: "2001:db8::1", ManagePort: 7500}); got != "http://[2001:db8::1]:7500" {
		t.Errorf("IPv6 地址要补方括号，实际 %q", got)
	}
	if got := dashBase(&model.Instance{ServerAddr: "frps.example.com", ManagePort: 7500}); got != "http://frps.example.com:7500" {
		t.Errorf("域名地址拼错：%q", got)
	}
}

// 编辑自己的代理：frps 表里那条同名代理不算冲突
func TestCheckPortFrpsAPIExcludesSelf(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.1", 7000, "")
	px := mustProxy(t, st, a.ID, "我自己的", "tcp", 8080, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"proxies":[{"name":"我自己的","conf":{"remotePort":8080}}]}`)
	}))
	defer srv.Close()
	setDash(t, st, a.ID, dashPort(t, srv.URL), "", "")

	excl := fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080,"exclude_proxy_id":%d}`, a.ID, px.ID)
	_, m := checkPort(t, h, excl)
	if m["state"] != "free" {
		t.Errorf("排除自身（同名）应 free，实际 %v", m)
	}
	// 同名但没排除 → 仍按占用提示，避免把「本机没登记但远端已有」的端口直接放行
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if m["state"] != "occupied" {
		t.Errorf("未排除自身应 occupied，实际 %v", m)
	}
}

func TestCheckPortFrpsAPIFailureIsUnknown(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.1", 7000, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	setDash(t, st, a.ID, dashPort(t, srv.URL), "dash", "错密码")

	_, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if m["state"] != "unknown" || !strings.Contains(m["note"], "鉴权失败") || !strings.Contains(m["note"], "不阻断") {
		t.Errorf("frps 鉴权失败应 unknown 并带上原因与不阻断说明，实际 %v", m)
	}

	// 管理端口没人应答（服务已关）同样只提示不拦
	srv.Close()
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":8080}`, a.ID))
	if m["state"] != "unknown" {
		t.Errorf("远端不可达应 unknown，实际 %v", m)
	}
}

func TestCheckPortUDP(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.1", 7000, "")
	mustProxy(t, st, a.ID, "dns", "udp", 53, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/proxy/udp" {
			fmt.Fprint(w, `{"proxies":[{"name":"dns","conf":{"remotePort":5353}}]}`)
			return
		}
		fmt.Fprint(w, `{"proxies":[]}`)
	}))
	defer srv.Close()
	setDash(t, st, a.ID, dashPort(t, srv.URL), "", "")

	// udp 本机查重命中 → occupied
	_, m := checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"udp","remote_port":53}`, a.ID))
	if m["state"] != "occupied" || !strings.Contains(m["holder"], "dns") {
		t.Fatalf("udp 查重命中应 occupied，实际 %v", m)
	}
	// udp 也能问 frps：表里有 → occupied（写名）
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"udp","remote_port":5353}`, a.ID))
	if m["state"] != "occupied" || !strings.Contains(m["holder"], "dns") {
		t.Errorf("udp 应能拿 frps 表判定，实际 %v", m)
	}
	// tcp 请求不受 udp 代理影响（协议不同不互斥），且查的是 tcp 表 → free
	_, m = checkPort(t, h, fmt.Sprintf(`{"instance_id":%d,"type":"tcp","remote_port":53}`, a.ID))
	if m["state"] != "free" {
		t.Errorf("tcp/udp 不应互相判占用，实际 %v", m)
	}
}

// ---------- 任务 5：代理保存服务端兜底查重 ----------

func createProxyVia(t *testing.T, h http.Handler, instanceID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, h, "POST", fmt.Sprintf("/api/instances/%d/proxies", instanceID), body)
}

func errText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("解析响应失败: %s", rec.Body.String())
	}
	return m["error"]
}

func TestCreateProxyRemotePortConflict(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "服务器A", "127.0.0.9", 7000, "")
	b := mustInstance(t, st, "服务器B", "127.0.0.9", 7000, "") // 同一 FRPS 服务器
	c := mustInstance(t, st, "服务器C", "127.0.0.8", 7000, "") // 另一台 FRPS

	rec := createProxyVia(t, h, a.ID, `{"name":"ssh","kind":"proxy","type":"tcp","local_port":22,"remote_port":8080}`)
	if rec.Code != 201 {
		t.Fatalf("首条 tcp 8080 应创建成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// 同服务器重复远程端口 → 400，文案带占用者代理名
	rec = createProxyVia(t, h, b.ID, `{"name":"backup","kind":"proxy","type":"tcp","local_port":22,"remote_port":8080}`)
	if rec.Code != 400 {
		t.Fatalf("重复远程端口应 400，实际 %d", rec.Code)
	}
	if got := errText(t, rec); got != "远程端口 8080 已被代理「ssh」占用" {
		t.Errorf("冲突文案不符: %q", got)
	}
	// 别的 FRPS 服务器不算冲突
	rec = createProxyVia(t, h, c.ID, `{"name":"ssh-c","kind":"proxy","type":"tcp","local_port":22,"remote_port":8080}`)
	if rec.Code != 201 {
		t.Errorf("不同 FRPS 服务器应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// udp 与 tcp 端口空间互不冲突
	rec = createProxyVia(t, h, a.ID, `{"name":"dns","kind":"proxy","type":"udp","local_port":53,"remote_port":8080}`)
	if rec.Code != 201 {
		t.Errorf("udp 8080 不受 tcp 8080 影响，应放行，实际 %d", rec.Code)
	}
	// http 类型不参与查重
	rec = createProxyVia(t, h, a.ID, `{"name":"web","kind":"proxy","type":"http","local_port":80,"remote_port":8080}`)
	if rec.Code != 201 {
		t.Errorf("http 类型不参与查重，应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// 禁用代理不算冲突
	rec = createProxyVia(t, h, a.ID, `{"name":"off","kind":"proxy","type":"tcp","local_port":9090,"remote_port":9090,"enabled":false}`)
	if rec.Code != 201 {
		t.Fatalf("禁用代理应创建成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	rec = createProxyVia(t, h, a.ID, `{"name":"on","kind":"proxy","type":"tcp","local_port":9090,"remote_port":9090}`)
	if rec.Code != 201 {
		t.Errorf("同端口仅有禁用代理时应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateProxyExcludesSelf(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	px := mustProxy(t, st, a.ID, "me", "tcp", 8080, true)
	// 直接落库另一条同端口启用代理：模拟“别的代理占着 8080”
	mustProxy(t, st, a.ID, "rival", "tcp", 8080, true)

	body8080 := `{"name":"me","kind":"proxy","type":"tcp","local_port":22,"remote_port":8080,"enabled":true}`
	rec := doJSON(t, h, "PUT", fmt.Sprintf("/api/proxies/%d", px.ID), body8080)
	if rec.Code != 400 {
		t.Fatalf("端口被他人占用时更新应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if got := errText(t, rec); got != "远程端口 8080 已被代理「rival」占用" {
		t.Errorf("冲突文案应指向 rival，实际 %q", got)
	}
	// 改到空闲端口 8081 → 放行；此时该端口唯一持有者是自身
	rec = doJSON(t, h, "PUT", fmt.Sprintf("/api/proxies/%d", px.ID),
		`{"name":"me","kind":"proxy","type":"tcp","local_port":22,"remote_port":8081,"enabled":true}`)
	if rec.Code != 200 {
		t.Fatalf("改到空闲端口应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// 再次原值提交 8081：查重必须排除自身 id，不应自撞
	rec = doJSON(t, h, "PUT", fmt.Sprintf("/api/proxies/%d", px.ID),
		`{"name":"me","kind":"proxy","type":"tcp","local_port":22,"remote_port":8081,"enabled":true}`)
	if rec.Code != 200 {
		t.Fatalf("排除自身后重复提交应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

// 禁用代理不写入 frpc 配置，保存时不该被端口查重拦住
func TestDisabledProxySkipsPortConflict(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	mustProxy(t, st, a.ID, "live", "tcp", 8080, true)
	rec := createProxyVia(t, h, a.ID, `{"name":"standby","kind":"proxy","type":"tcp","local_port":22,"remote_port":8080,"enabled":false}`)
	if rec.Code != 201 {
		t.Fatalf("禁用代理应放行，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

// 启用开关把代理推给 frps，端口已被占用时必须拦住启用
func TestEnableProxyRejectsTakenPort(t *testing.T) {
	_, st, _, h := newTestServer(t)
	a := mustInstance(t, st, "A", "127.0.0.9", 7000, "")
	standby := mustProxy(t, st, a.ID, "standby", "tcp", 9090, false)
	mustProxy(t, st, a.ID, "live", "tcp", 9090, true)
	rec := doJSON(t, h, "PUT", fmt.Sprintf("/api/proxies/%d/enabled", standby.ID), `{"enabled":true}`)
	if rec.Code != 400 {
		t.Fatalf("端口被占用时启用应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if got := errText(t, rec); got != "远程端口 9090 已被代理「live」占用" {
		t.Errorf("启用拦截文案不符: %q", got)
	}
	if cur, _ := st.GetProxy(standby.ID); cur.Enabled {
		t.Error("被拦下后代理不应被置为启用")
	}
	// 停用永远放行
	if rec := doJSON(t, h, "PUT", fmt.Sprintf("/api/proxies/%d/enabled", standby.ID), `{"enabled":false}`); rec.Code != 200 {
		t.Errorf("停用应放行，实际 %d", rec.Code)
	}
}

// 指定版本必须是 x.y.z，避免脏值进库
func TestInstanceRejectsBadFrpcVersion(t *testing.T) {
	_, _, _, h := newTestServer(t)
	for _, bad := range []string{"1.2", "v0.61.1", "0.61.1-beta", "abc"} {
		body := fmt.Sprintf(`{"name":"n-%s","server_addr":"1.2.3.4","server_port":7000,"frpc_version":%q}`, bad, bad)
		if rec := doJSON(t, h, "POST", "/api/instances", body); rec.Code != 400 {
			t.Errorf("版本 %q 应 400，实际 %d: %s", bad, rec.Code, rec.Body.String())
		}
	}
	if rec := doJSON(t, h, "POST", "/api/instances", `{"name":"ok","server_addr":"1.2.3.4","server_port":7000,"frpc_version":"0.61.1"}`); rec.Code != 201 {
		t.Errorf("合法版本应创建成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
}
