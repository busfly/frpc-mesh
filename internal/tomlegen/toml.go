package tomlegen

import (
	"strconv"
	"strings"

	"frpc-mesh/internal/model"
)

func q(s string) string { return strconv.Quote(s) }

// withDefaults 为零值的保活参数填入实测稳定值
func withDefaults(t model.Transport) model.Transport {
	if t.Heartbeat <= 0 {
		t.Heartbeat = 30
	}
	if t.HeartbeatTimeout <= 0 {
		t.HeartbeatTimeout = 90
	}
	if t.TcpMuxKeepalive <= 0 {
		t.TcpMuxKeepalive = 30
	}
	if t.DialTimeout <= 0 {
		t.DialTimeout = 10
	}
	if t.DialServerKeepalive <= 0 {
		t.DialServerKeepalive = 7200
	}
	return t
}

func strArray(csv string) string {
	parts := []string{}
	for _, p := range strings.Split(csv, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, q(p))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Generate 生成一个实例（远程 frps）对应的完整 frpc TOML 配置
func Generate(in *model.Instance, proxies []*model.Proxy) string {
	var b strings.Builder
	b.WriteString("# 由 frpc-mesh 自动生成\n")
	b.WriteString("serverAddr = " + q(in.ServerAddr) + "\n")
	b.WriteString("serverPort = " + strconv.Itoa(in.ServerPort) + "\n")
	// 登录失败不退出进程，保持重试，避免 frps 短暂不可达时实例永久停摆
	b.WriteString("loginFailExit = false\n\n")

	if in.AuthMethod == "token" {
		b.WriteString("auth.method = \"token\"\n")
		b.WriteString("auth.token = " + q(in.AuthToken) + "\n\n")
	}

	t := withDefaults(in.Transport)
	if t.Protocol != "" && t.Protocol != "tcp" {
		b.WriteString("transport.protocol = " + q(t.Protocol) + "\n")
	}
	if t.TCPMux {
		b.WriteString("transport.tcpMux = true\n")
	}
	if t.PoolCount > 0 {
		b.WriteString("transport.poolCount = " + strconv.Itoa(t.PoolCount) + "\n")
	}
	b.WriteString("transport.heartbeatInterval = " + strconv.Itoa(t.Heartbeat) + "\n")
	b.WriteString("transport.heartbeatTimeout = " + strconv.Itoa(t.HeartbeatTimeout) + "\n")
	b.WriteString("transport.tcpMuxKeepaliveInterval = " + strconv.Itoa(t.TcpMuxKeepalive) + "\n")
	b.WriteString("transport.dialServerTimeout = " + strconv.Itoa(t.DialTimeout) + "\n")
	b.WriteString("transport.dialServerKeepalive = " + strconv.Itoa(t.DialServerKeepalive) + "\n")
	if t.ProxyProtocolVersion != "" {
		b.WriteString("transport.proxyProtocolVersion = " + q(t.ProxyProtocolVersion) + "\n")
	}
	if t.CustomStunServer != "" {
		b.WriteString("transport.customStunServer = " + q(t.CustomStunServer) + "\n")
	}
	if t.TLSEnable {
		b.WriteString("transport.tls.enable = true\n")
	}
	b.WriteString("\n")

	a := in.AdminCfg
	if a.Enabled && a.Port > 0 {
		b.WriteString("webServer.addr = \"127.0.0.1\"\n")
		b.WriteString("webServer.port = " + strconv.Itoa(a.Port) + "\n")
		if a.User != "" {
			b.WriteString("webServer.user = " + q(a.User) + "\n")
		}
		if a.Password != "" {
			b.WriteString("webServer.password = " + q(a.Password) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("log.to = \"console\"\n")
	b.WriteString("log.level = \"info\"\n")

	for _, p := range proxies {
		if p.Kind == "visitor" {
			writeVisitor(&b, p)
		} else {
			writeProxy(&b, p)
		}
	}
	return b.String()
}

func writeProxy(b *strings.Builder, p *model.Proxy) {
	b.WriteString("\n[[proxies]]\n")
	b.WriteString("name = " + q(p.Name) + "\n")
	b.WriteString("type = " + q(p.Type) + "\n")
	switch p.Type {
	case "tcp", "udp":
		writeLocal(b, p)
		if p.RemotePort > 0 {
			b.WriteString("remotePort = " + strconv.Itoa(p.RemotePort) + "\n")
		}
	case "http", "https":
		writeLocal(b, p)
		if p.CustomDomains != "" {
			b.WriteString("customDomains = " + strArray(p.CustomDomains) + "\n")
		}
		if p.Subdomain != "" {
			b.WriteString("subdomain = " + q(p.Subdomain) + "\n")
		}
		if p.Locations != "" {
			b.WriteString("locations = " + strArray(p.Locations) + "\n")
		}
		if p.HTTPUser != "" {
			b.WriteString("httpUser = " + q(p.HTTPUser) + "\n")
		}
		if p.HTTPPassword != "" {
			b.WriteString("httpPassword = " + q(p.HTTPPassword) + "\n")
		}
		if p.HostHeaderRewrite != "" {
			b.WriteString("hostHeaderRewrite = " + q(p.HostHeaderRewrite) + "\n")
		}
	case "stcp", "sudp", "xtcp":
		writeLocal(b, p)
		if p.SecretKey != "" {
			b.WriteString("secretKey = " + q(p.SecretKey) + "\n")
		}
	}
	writeTransport(b, p)
}

func writeVisitor(b *strings.Builder, p *model.Proxy) {
	b.WriteString("\n[[visitors]]\n")
	b.WriteString("name = " + q(p.Name) + "\n")
	b.WriteString("type = " + q(p.Type) + "\n")
	if p.ServerName != "" {
		b.WriteString("serverName = " + q(p.ServerName) + "\n")
	}
	if p.SecretKey != "" {
		b.WriteString("secretKey = " + q(p.SecretKey) + "\n")
	}
	bindAddr := p.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	b.WriteString("bindAddr = " + q(bindAddr) + "\n")
	if p.BindPort > 0 {
		b.WriteString("bindPort = " + strconv.Itoa(p.BindPort) + "\n")
	}
}

func writeLocal(b *strings.Builder, p *model.Proxy) {
	ip := p.LocalIP
	if ip == "" {
		ip = "127.0.0.1"
	}
	b.WriteString("localIP = " + q(ip) + "\n")
	if p.LocalPort > 0 {
		b.WriteString("localPort = " + strconv.Itoa(p.LocalPort) + "\n")
	}
}

func writeTransport(b *strings.Builder, p *model.Proxy) {
	if p.UseEncryption {
		b.WriteString("transport.useEncryption = true\n")
	}
	if p.UseCompression {
		b.WriteString("transport.useCompression = true\n")
	}
	if p.BandwidthLimit != "" {
		b.WriteString("transport.bandwidthLimit = " + q(p.BandwidthLimit) + "\n")
		b.WriteString("transport.bandwidthLimitMode = \"client\"\n")
	}
}
