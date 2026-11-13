package frpsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Auth frps 管理接口（webServer）的凭据，对应 frps.toml 的 webServer.user / webServer.password。
// 核对过 frp v0.61.1 与 dev 的源码：dashboard 只认 HTTP Basic（两者都为空时匿名放行），没有 Bearer 分支。
type Auth struct {
	User     string
	Password string
}

// 检测在表单里是同步等的动作，留 3 秒足够跨公网一次往返
const timeout = 3 * time.Second

// 必须直连 frps：本进程常设 HTTPS_PROXY 供 GitHub 下载用，走代理等于让代理替 frps 作答，
// 拿到的端口表就不可信了（代理返回空 200 会被当成「一个代理都没有」）
var client = &http.Client{Transport: &http.Transport{Proxy: nil}}

type proxyEntry struct {
	Name string `json:"name"`
	Conf struct {
		RemotePort int `json:"remotePort"`
	} `json:"conf"`
}

// UsedPorts 拉这台 frps 已登记的远程端口，返回 端口号 → frps 侧代理名。
// typ 只支持 tcp/udp（http/https 走域名不占独立端口）。
// 取不到就返回错误，由调用方降级为「无法确定」——这里绝不把「拿不到」说成「空闲」或「占用」。
func UsedPorts(ctx context.Context, base string, a Auth, typ string) (map[int]string, error) {
	if typ != "tcp" && typ != "udp" {
		return nil, fmt.Errorf("仅支持 tcp/udp 的端口查询")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/proxy/"+typ, nil)
	if err != nil {
		return nil, err
	}
	if a.User != "" {
		req.SetBasicAuth(a.User, a.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("frps 管理接口鉴权失败（HTTP %d），请核对 frps 的 webServer.user / webServer.password", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frps 管理接口返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	entries, err := parseEntries(body)
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(entries))
	for _, e := range entries {
		if e.Conf.RemotePort > 0 {
			out[e.Conf.RemotePort] = e.Name
		}
	}
	return out, nil
}

// parseEntries 只认 frps 真实返回的 {"proxies":[...]}（v0.61 的 GetProxyInfoResp、dev 的 model 同名结构）。
// 「没有代理」只由 proxies 为 [] 或 null 表达；空响应体、顶层 null、缺字段、形状对不上都必须报错——
// 解析失败绝不能退化成空表，那等于把「问不到」说成「端口空闲」，正是这次要修掉的那类误判。
func parseEntries(body []byte) ([]proxyEntry, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(body), &obj); err != nil {
		return nil, fmt.Errorf("解析 frps 管理接口响应失败: %w", err)
	}
	raw, ok := obj["proxies"]
	if !ok {
		return nil, errors.New("frps 管理接口响应里没有 proxies 字段，无法判定端口占用")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var entries []proxyEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("解析 frps 管理接口响应失败: %w", err)
	}
	return entries, nil
}
