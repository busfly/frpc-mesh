package api

import (
	"encoding/json"
	"testing"

	"frpc-mesh/internal/model"
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
