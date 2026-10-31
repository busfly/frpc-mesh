package api

import "testing"

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
