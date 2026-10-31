package runner

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// ---------- 任务 3：指定版本补下载与自动切换 ----------

func newEnabledInstance(t *testing.T, st *store.Store, name, pin string) *model.Instance {
	t.Helper()
	in := &model.Instance{Name: name, ServerAddr: "127.0.0.1", ServerPort: 7000,
		AuthMethod: "token", Enabled: true, FrpcVersion: pin}
	if err := st.CreateInstance(in); err != nil {
		t.Fatal(err)
	}
	return in
}

// fakeDownload 用注入的 downloadFunc 模拟异步下载任务：先登记 downloading，
// delay 后翻到 final（done/error），watcher 轮询即可到终态，测试不碰真实网络
func fakeDownload(m *Manager, final string, delay time.Duration, calls *[]string) {
	m.downloadFunc = func(version, source string) error {
		job := &downloadJob{status: "downloading"}
		m.mu.Lock()
		m.jobs[version] = job
		m.mu.Unlock()
		*calls = append(*calls, version+"|"+source)
		go func() {
			time.Sleep(delay)
			job.mu.Lock()
			job.status = final
			job.mu.Unlock()
		}()
		return nil
	}
	m.watchPoll = 2 * time.Millisecond
}

type restartLog struct {
	mu  sync.Mutex
	ids []int64
}

func (l *restartLog) fn(id int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ids = append(l.ids, id)
	return nil
}

func (l *restartLog) snapshot() []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]int64{}, l.ids...)
}

func (m *Manager) isDownloadFailed(version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, failed := m.downloadFailed[version]
	return failed
}

// markFakeRunning 不真起进程，直接在 procs 里登记运行态，供 IsRunning 判定
func markFakeRunning(t *testing.T, m *Manager, id int64) {
	t.Helper()
	m.mu.Lock()
	m.procs[id] = &proc{status: StatusRunning, done: make(chan struct{})}
	m.mu.Unlock()
	t.Cleanup(func() {
		m.mu.Lock()
		delete(m.procs, id)
		m.mu.Unlock()
	})
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func TestReconcileTriggersMissingPinDownload(t *testing.T) {
	m, st := newTestManager(t)
	var calls []string
	fakeDownload(m, "error", 10*time.Millisecond, &calls)
	log := &restartLog{}
	m.restartFunc = log.fn
	newEnabledInstance(t, st, "机A", "0.66.0")

	m.ReconcilePinnedVersions()
	if len(calls) != 1 || calls[0] != "0.66.0|github" {
		t.Fatalf("pin 缺失应触发一次 github 补下载，实际 %v", calls)
	}
	waitUntil(t, func() bool { return m.isDownloadFailed("0.66.0") })
	if got := log.snapshot(); len(got) != 0 {
		t.Errorf("下载失败不得重启，实际 %v", got)
	}
}

func TestReconcileSkipsDisabledAndDownloaded(t *testing.T) {
	m, st := newTestManager(t)
	var calls []string
	fakeDownload(m, "done", 10*time.Millisecond, &calls)
	writeFakeFrpc(t, m, "0.61.0")

	dis := &model.Instance{Name: "停用机", ServerAddr: "127.0.0.1", ServerPort: 7000,
		AuthMethod: "token", Enabled: false, FrpcVersion: "0.66.0"}
	if err := st.CreateInstance(dis); err != nil {
		t.Fatal(err)
	}
	newEnabledInstance(t, st, "已下载机", "0.61.0")
	newEnabledInstance(t, st, "跟随默认机", "")

	m.ReconcilePinnedVersions()
	if len(calls) != 0 {
		t.Errorf("未启用/已下载/未指定都不该触发补下载，实际 %v", calls)
	}
}

func TestReconcileDedupsSameVersion(t *testing.T) {
	m, st := newTestManager(t)
	var calls []string
	fakeDownload(m, "error", 50*time.Millisecond, &calls)
	newEnabledInstance(t, st, "机A", "0.66.0")
	newEnabledInstance(t, st, "机B", "0.66.0")

	m.ReconcilePinnedVersions()
	if len(calls) != 1 {
		t.Errorf("同版本只应触发一次下载，实际 %v", calls)
	}
	waitUntil(t, func() bool { return m.isDownloadFailed("0.66.0") })
}

func TestReconcileRestartsRunningAfterSuccess(t *testing.T) {
	m, st := newTestManager(t)
	var calls []string
	fakeDownload(m, "done", 10*time.Millisecond, &calls)
	log := &restartLog{}
	m.restartFunc = log.fn

	running := newEnabledInstance(t, st, "运行中的机", "0.66.0")
	stopped := newEnabledInstance(t, st, "没跑的机", "0.66.0")
	other := newEnabledInstance(t, st, "别的版本", "0.67.0")
	markFakeRunning(t, m, running.ID)

	m.ReconcilePinnedVersions()
	waitUntil(t, func() bool { return len(log.snapshot()) >= 1 })
	got := log.snapshot()
	if len(got) != 1 || got[0] != running.ID {
		t.Fatalf("只应重启 pin==该版本且运行中的实例，实际 %v（running=%d stopped=%d other=%d）",
			got, running.ID, stopped.ID, other.ID)
	}
}

func TestReconcileFailureNoRestartNoRetry(t *testing.T) {
	m, st := newTestManager(t)
	var calls []string
	fakeDownload(m, "error", 10*time.Millisecond, &calls)
	log := &restartLog{}
	m.restartFunc = log.fn

	running := newEnabledInstance(t, st, "机A", "0.66.0")
	markFakeRunning(t, m, running.ID)

	m.ReconcilePinnedVersions()
	waitUntil(t, func() bool { return m.isDownloadFailed("0.66.0") })
	if got := log.snapshot(); len(got) != 0 {
		t.Errorf("下载失败不得重启，实际 %v", got)
	}
	// 本进程内不再重试（等首个 watcher 退出后再 reconcile）
	time.Sleep(20 * time.Millisecond)
	m.ReconcilePinnedVersions()
	if len(calls) != 1 {
		t.Errorf("失败后不应重试下载，实际 %v", calls)
	}
}
