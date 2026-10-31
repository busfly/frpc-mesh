package netinfo

import (
	"encoding/hex"
	"net"
	"os"
	"strings"
)

type Hints struct {
	HostDockerInternal string `json:"host_docker_internal"` // 可解析时为 "host.docker.internal"，容器访问宿主机的首选地址
	HostGateway        string `json:"host_gateway"`         // 容器内到达宿主机的网桥网关，非容器环境为空
	ContainerIP        string `json:"container_ip"`         // 本机出口 IP
	Source             string `json:"source"`               // env | proc | none
}

// Detect 探测常用本地地址提示：优先 host.docker.internal 域名，其次默认路由网关
func Detect() Hints {
	h := Hints{Source: "none"}
	if addrs, err := net.LookupHost("host.docker.internal"); err == nil && len(addrs) > 0 {
		h.HostDockerInternal = "host.docker.internal"
	}
	if v := os.Getenv("FRPC_MESH_HOST_GATEWAY"); v != "" {
		h.HostGateway = v
		h.Source = "env"
	} else if g := gatewayFromProc(); g != "" {
		h.HostGateway = g
		h.Source = "proc"
	}
	h.ContainerIP = outboundIP()
	return h
}

func gatewayFromProc() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if f[1] == "00000000" { // 默认路由
			raw, err := hex.DecodeString(f[2])
			if err != nil || len(raw) != 4 {
				continue
			}
			gw := net.IPv4(raw[3], raw[2], raw[1], raw[0]) // 小端
			if gw.IsLoopback() || gw.IsUnspecified() {
				continue
			}
			return gw.String()
		}
	}
	return ""
}

func outboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}
