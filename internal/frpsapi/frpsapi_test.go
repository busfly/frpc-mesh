package frpsapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// frps 真实响应是对象包裹：{"proxies":[...]}（v0.61 的 GetProxyInfoResp 与 dev 的 model 同名结构都是它），
// 所有夹具都按这个形状写，别用裸数组给自己造假的通过理由
func TestUsedPortsBasicAuth(t *testing.T) {
	var gotPath, gotUser string
	var basicOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, _, basicOK = r.BasicAuth()
		fmt.Fprint(w, `{"proxies":[{"name":"web","conf":{"remotePort":8080}},{"name":"api","conf":{"remotePort":8081}}]}`)
	}))
	defer srv.Close()

	ports, err := UsedPorts(context.Background(), srv.URL, Auth{User: "admin", Password: "pw"}, "tcp")
	if err != nil {
		t.Fatalf("应成功，实际 %v", err)
	}
	if gotPath != "/api/proxy/tcp" {
		t.Errorf("路径应为 /api/proxy/tcp，实际 %q", gotPath)
	}
	if !basicOK || gotUser != "admin" {
		t.Errorf("应带 Basic 凭据 admin，实际 ok=%v user=%q", basicOK, gotUser)
	}
	if ports[8080] != "web" || ports[8081] != "api" || len(ports) != 2 {
		t.Errorf("端口映射不对，实际 %v", ports)
	}
}

func TestUsedPortsUDPPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"proxies":[{"name":"dns","conf":{"remotePort":5353},"status":"online"}]}`)
	}))
	defer srv.Close()
	ports, err := UsedPorts(context.Background(), srv.URL, Auth{}, "udp")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/proxy/udp" || ports[5353] != "dns" {
		t.Errorf("udp 应查 /api/proxy/udp，实际 path=%q ports=%v", gotPath, ports)
	}
}

// 「一台代理都没有」只由 proxies 为 [] 或 null 表达
func TestUsedPortsEmptyProxiesObject(t *testing.T) {
	for _, body := range []string{`{"proxies":[]}`, `{"proxies":null}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		ports, err := UsedPorts(context.Background(), srv.URL, Auth{}, "udp")
		srv.Close()
		if err != nil || len(ports) != 0 {
			t.Errorf("空表 %s 应解析为空映射，实际 ports=%v err=%v", body, ports, err)
		}
	}
}

// 形状不对时必须报错，绝不能当成「端口空闲」放过去。
// 空响应体与顶层 null 尤其危险：拦截代理/打错的地址常返回空 200，认成空表就等于给出绿色的「端口空闲」
func TestUsedPortsUnexpectedShapeIsError(t *testing.T) {
	for _, body := range []string{`{"code":200,"msg":"ok"}`, `null`, ``, `[{"name":"x","conf":{"remotePort":1}}]`, `{"proxies":{}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		_, err := UsedPorts(context.Background(), srv.URL, Auth{}, "tcp")
		srv.Close()
		if err == nil {
			t.Errorf("响应 %q 不是 frps 的端口表，应报错而不是判成空闲", body)
		}
	}
}

func TestUsedPortsNoCredentialMeansAnonymous(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"proxies":[]}`)
	}))
	defer srv.Close()

	ports, err := UsedPorts(context.Background(), srv.URL, Auth{}, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Errorf("未填凭据不应带 Authorization，实际 %q", auth)
	}
	if len(ports) != 0 {
		t.Errorf("空表应为空映射，实际 %v", ports)
	}
}

// 本进程为下载 frpc 常设 HTTP(S)_PROXY，端口查询必须绕开它直连 frps，
// 否则代理替 frps 作答（返回空 200 就会变成「端口空闲」）
func TestUsedPortsIgnoresEnvProxy(t *testing.T) {
	var proxied int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied++
		fmt.Fprint(w, `{"proxies":[]}`)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"proxies":[{"name":"直连才看得到","conf":{"remotePort":22001}}]}`)
	}))
	defer srv.Close()

	ports, err := UsedPorts(context.Background(), srv.URL, Auth{}, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if proxied != 0 || ports[22001] != "直连才看得到" {
		t.Errorf("应直连 frps 不走环境代理，实际 proxied=%d ports=%v", proxied, ports)
	}
}

func TestUsedPortsErrorsAreNotSilent(t *testing.T) {
	srv401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv401.Close()
	if _, err := UsedPorts(context.Background(), srv401.URL, Auth{User: "u"}, "tcp"); err == nil ||
		!strings.Contains(err.Error(), "鉴权失败") {
		t.Errorf("401 应报鉴权失败，实际 %v", err)
	}

	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srvBad.Close()
	if _, err := UsedPorts(context.Background(), srvBad.URL, Auth{}, "tcp"); err == nil {
		t.Error("坏响应应报错，不能当成空闲")
	}

	if _, err := UsedPorts(context.Background(), "", Auth{}, "tcp"); err == nil {
		t.Error("没配管理地址应报错")
	}
	if _, err := UsedPorts(context.Background(), "http://x", Auth{}, "http"); err == nil {
		t.Error("http 类型不该走端口查询")
	}
}

func TestUsedPortsIgnoresZeroRemotePort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"proxies":[{"name":"随机端口","conf":{"remotePort":0}},{"name":"无 conf"}]}`)
	}))
	defer srv.Close()
	ports, err := UsedPorts(context.Background(), srv.URL, Auth{}, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 0 {
		t.Errorf("remotePort=0（随机分配）与缺 conf 都不该进表，实际 %v", ports)
	}
}
