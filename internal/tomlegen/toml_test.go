package tomlegen

import (
	"strings"
	"testing"

	"frpc-mesh/internal/model"
)

func TestGenerate(t *testing.T) {
	in := &model.Instance{
		Name: "test", ServerAddr: "1.2.3.4", ServerPort: 7000,
		AuthMethod: "token", AuthToken: `tok"en`,
		Transport: model.Transport{Protocol: "tcp", TCPMux: true, PoolCount: 2, Heartbeat: 30, DialTimeout: 10, TLSEnable: true},
		AdminCfg:  model.Admin{Enabled: true, Port: 17400, User: "u", Password: "p"},
	}
	proxies := []*model.Proxy{
		{Kind: "proxy", Name: "ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 6000, UseEncryption: true, UseCompression: true},
		{Kind: "proxy", Name: "web", Type: "http", LocalIP: "192.168.1.2", LocalPort: 8080, CustomDomains: "a.com, b.com", Subdomain: "sub", HTTPUser: "hu", HTTPPassword: "hp", HostHeaderRewrite: "x"},
		{Kind: "proxy", Name: "p2p", Type: "stcp", LocalIP: "127.0.0.1", LocalPort: 3000, SecretKey: "sk"},
		{Kind: "visitor", Name: "p2p-v", Type: "stcp", ServerName: "p2p", SecretKey: "sk", BindAddr: "127.0.0.1", BindPort: 9000},
	}
	got := Generate(in, proxies)

	mustContain := []string{
		`serverAddr = "1.2.3.4"`,
		`serverPort = 7000`,
		`loginFailExit = false`,
		`auth.token = "tok\"en"`,
		`transport.tcpMux = true`,
		`transport.poolCount = 2`,
		`transport.heartbeatInterval = 30`,
		`transport.heartbeatTimeout = 90`,
		`transport.tcpMuxKeepaliveInterval = 30`,
		`transport.dialServerKeepalive = 7200`,
		`transport.tls.enable = true`,
		`webServer.port = 17400`,
		`[[proxies]]`,
		`[[visitors]]`,
		`customDomains = ["a.com", "b.com"]`,
		`transport.useEncryption = true`,
		`secretKey = "sk"`,
		`serverName = "p2p"`,
		`bindPort = 9000`,
	}
	for _, m := range mustContain {
		if !strings.Contains(got, m) {
			t.Errorf("缺少片段: %s\n生成结果:\n%s", m, got)
		}
	}
	if strings.Count(got, "[[proxies]]") != 3 || strings.Count(got, "[[visitors]]") != 1 {
		t.Errorf("proxies/visitors 数量错误:\n%s", got)
	}
	if strings.Contains(got, "transport.protocol") {
		t.Errorf("tcp 协议不应显式输出 transport.protocol")
	}
}

func TestGenerateNoAuth(t *testing.T) {
	in := &model.Instance{Name: "n", ServerAddr: "h", ServerPort: 7000, AuthMethod: "none"}
	got := Generate(in, nil)
	if strings.Contains(got, "auth.") {
		t.Errorf("none 认证不应输出 auth 段: %s", got)
	}
}
