package runner

import (
	"os"
	"path/filepath"
	"testing"

	"frpc-mesh/internal/model"
	"frpc-mesh/internal/store"
)

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;34m2026-10-31 [I] start frpc\x1b[0m": "2026-10-31 [I] start frpc",
		"plain line without color":                   "plain line without color",
		"[0m [1;34m2026-10-31 login success\x1b[0m":  "[0m [1;34m2026-10-31 login success",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func newTestManager(t *testing.T) (*Manager, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "frpc-mesh.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	m := NewManager(st, t.TempDir())
	t.Cleanup(func() {
		m.StopAll()
		st.Close()
	})
	return m, st
}

// writeFakeFrpc 造一个假的“已下载”版本；sleep 脚本让子进程在测试期间保持存活
func writeFakeFrpc(t *testing.T, m *Manager, version string) {
	t.Helper()
	p := filepath.Join(m.VersionsDir(), "frpc-"+version)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("写入假 frpc 失败: %v", err)
	}
}

func TestResolveFrpcForPriority(t *testing.T) {
	m, st := newTestManager(t)
	writeFakeFrpc(t, m, "0.60.0")
	writeFakeFrpc(t, m, "0.61.0")
	if err := st.SetSetting("frpc_version", "0.60.0"); err != nil {
		t.Fatal(err)
	}

	// pin 已下载优先于默认版本
	path, eff, err := m.ResolveFrpcFor(&model.Instance{FrpcVersion: "0.61.0"})
	if err != nil || eff != "0.61.0" || path != m.versionPath("0.61.0") {
		t.Errorf("pin 已下载应返回 pin: path=%q eff=%q err=%v", path, eff, err)
	}
	// pin 未下载回落默认版本
	path, eff, err = m.ResolveFrpcFor(&model.Instance{FrpcVersion: "0.99.0"})
	if err != nil || eff != "0.60.0" || path != m.versionPath("0.60.0") {
		t.Errorf("pin 缺失应回落默认: path=%q eff=%q err=%v", path, eff, err)
	}
	// 未指定用默认版本
	if _, eff, err := m.ResolveFrpcFor(&model.Instance{}); err != nil || eff != "0.60.0" {
		t.Errorf("未指定应使用默认: eff=%q err=%v", eff, err)
	}
}

func TestResolveFrpcForFallback(t *testing.T) {
	m, _ := newTestManager(t)
	// 无激活版本、pin 未下载 → 走系统 frpc 兜底；effective 必须为空串（版本未知），
	// 路径断言与 resolveFrpcBinary 探测结果对齐，不写死本机路径
	base, baseErr := m.resolveFrpcBinary()
	path, eff, err := m.ResolveFrpcFor(&model.Instance{FrpcVersion: "0.99.0"})
	if eff != "" {
		t.Errorf("兜底分支 effective 应为空，实际 %q", eff)
	}
	if (err == nil) != (baseErr == nil) || (err == nil && path != base) {
		t.Errorf("兜底应与 resolveFrpcBinary 一致: path=%q base=%q err=%v baseErr=%v", path, base, err, baseErr)
	}
}

func TestStartRecordsRuntimeFrpcVersion(t *testing.T) {
	m, st := newTestManager(t)
	writeFakeFrpc(t, m, "0.60.0")
	writeFakeFrpc(t, m, "0.61.0")
	if err := st.SetSetting("frpc_version", "0.60.0"); err != nil {
		t.Fatal(err)
	}

	in := &model.Instance{Name: "固定版本机", ServerAddr: "127.0.0.1", ServerPort: 7000,
		AuthMethod: "token", FrpcVersion: "0.61.0"}
	if err := st.CreateInstance(in); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(in); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if rt := m.Runtime(in.ID); rt.FrpcVersion != "0.61.0" {
		t.Errorf("Runtime 应记录 pin 版本，实际 %q", rt.FrpcVersion)
	}
	if err := m.Stop(in.ID); err != nil {
		t.Fatal(err)
	}

	in2 := &model.Instance{Name: "跟随默认机", ServerAddr: "127.0.0.1", ServerPort: 7000,
		AuthMethod: "token"}
	if err := st.CreateInstance(in2); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(in2); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if rt := m.Runtime(in2.ID); rt.FrpcVersion != "0.60.0" {
		t.Errorf("未 pin 应记录默认版本，实际 %q", rt.FrpcVersion)
	}
	if err := m.Stop(in2.ID); err != nil {
		t.Fatal(err)
	}
}
