package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"frpc-mesh/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "frpc-mesh.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func releaseJSON(tag, name string, assets []string) string {
	parts := make([]string, 0, len(assets))
	for _, a := range assets {
		parts = append(parts, fmt.Sprintf(`{"name":%q,"size":123,"browser_download_url":"https://github.com/%s"}`, a, a))
	}
	return fmt.Sprintf(`{"tag_name":%q,"name":%q,"html_url":"https://github.com/busfly/frpc-mesh/releases/tag/%s",
		"published_at":"2026-10-31T10:00:00Z","body":"## 更新内容","assets":[%s]}`, tag, name, tag, strings.Join(parts, ","))
}

func TestHasUpdate(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.1", true},
		{"v1.0.0", "1.0.1", true},
		{"1.0.0", "1.0.0", false},
		{"1.1.0", "1.0.9", false},
		{"1.9.0", "1.10.0", true},
		{"1.0.0", "", false},
		{"1.0.0-dev", "1.0.1", false},   // 非正式版本不做比较，避免误报
		{"1.0.0", "2.0.0-beta", false}, // 预发布不提示
	}
	for _, c := range cases {
		if got := hasUpdate(c.current, c.latest); got != c.want {
			t.Errorf("hasUpdate(%q,%q)=%v，期望 %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestResultParsesLatestAndPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releaseJSON("v1.2.3", "frpc-mesh v1.2.3", []string{assetNameFor("1.2.3"), checksums}))
	}))
	defer srv.Close()

	st := newStore(t)
	c := New(st, "1.0.0")
	c.apiURL = srv.URL
	c.mirrorURL = srv.URL

	inf := c.Result("github")
	if inf.Error != "" {
		t.Fatalf("检查失败: %s", inf.Error)
	}
	if inf.Latest != "1.2.3" || !inf.HasUpdate {
		t.Errorf("最新版本 %q / has_update=%v，期望 1.2.3 / true", inf.Latest, inf.HasUpdate)
	}
	if len(inf.Assets) != 2 {
		t.Errorf("资产数 %d，期望 2", len(inf.Assets))
	}
	if inf.CheckedAt == "" {
		t.Error("checked_at 为空")
	}

	// 换一个新 Checker（模拟重启）后仍应读到落盘结果，且不发请求
	c2 := New(st, "1.0.0")
	got := c2.Status()
	if got.Latest != "1.2.3" || !got.HasUpdate {
		t.Errorf("重启后状态丢失: latest=%q has_update=%v", got.Latest, got.HasUpdate)
	}
	if !got.Auto {
		t.Error("auto_check 默认应为开启")
	}
}

func TestResultFallsBackToMirror(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releaseJSON("v1.2.3", "x", nil))
	}))
	defer srv.Close()

	c := New(newStore(t), "1.0.0")
	c.apiURL = "https://127.0.0.1:1/unreachable"
	c.mirrorURL = srv.URL
	inf := c.Result("github")
	if inf.Source != "ghfast" {
		t.Errorf("应回退到镜像源，实际 source=%q err=%q", inf.Source, inf.Error)
	}
	if inf.Latest != "1.2.3" {
		t.Errorf("镜像源解析失败: latest=%q err=%q", inf.Latest, inf.Error)
	}
}

func TestResultRecordsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(newStore(t), "1.0.0")
	c.apiURL = srv.URL
	c.mirrorURL = srv.URL
	inf := c.Result("github")
	if inf.Error == "" {
		t.Error("接口 403 时应记录 error")
	}
	if inf.Latest != "" || inf.HasUpdate {
		t.Error("检查失败不应产生新版本结论")
	}
}

func TestCapabilityModes(t *testing.T) {
	realContainer := inContainer
	defer func() { inContainer = realContainer }()

	u := NewUpgrader(New(newStore(t), "1.0.0"), &fakeStop{}, t.TempDir())
	asset := assetNameFor("1.2.3")
	full := []Asset{{Name: asset}, {Name: checksums}}

	inContainer = func() bool { return false }
	if cap := u.CapabilityFor("1.2.3", full); !cap.CanUpgrade || cap.Mode != "binary" {
		t.Errorf("可写二进制环境应可升级: %+v", cap)
	}
	if cap := u.CapabilityFor("1.2.3", []Asset{{Name: asset}}); cap.CanUpgrade || cap.Mode != "unsupported" {
		t.Errorf("缺校验和应拒绝自动升级: %+v", cap)
	}
	if cap := u.CapabilityFor("1.2.3", []Asset{{Name: "frpc-mesh-v1.2.3-plan9-arm.tar.gz"}, {Name: checksums}}); cap.CanUpgrade {
		t.Error("当前平台无安装包时不应可升级")
	}
	if cap := u.CapabilityFor("", nil); cap.Reason == "" {
		t.Error("未检查到版本时应给出原因")
	}

	inContainer = func() bool { return true }
	cap := u.CapabilityFor("1.2.3", full)
	if cap.CanUpgrade || cap.Mode != "container" || !strings.Contains(cap.Reason, "容器") {
		t.Errorf("容器环境应只提示不自动升级: %+v", cap)
	}
}

type fakeStop struct{ n int }

func (f *fakeStop) StopAll() { f.n++ }

// fakePackage 构造一个"可执行的新版本二进制"打包成 tar.gz：--version 输出 v<ver>
func fakePackage(t *testing.T, version string) []byte {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\necho v%s\n", version)

	var raw bytes.Buffer
	zw := gzip.NewWriter(&raw)
	tw := tar.NewWriter(zw)
	// 目录前缀模拟真实发布包结构，同时验证解包只按文件名取主程序
	hdr := &tar.Header{Name: "frpc-mesh-v" + version + "-" + runtime.GOOS + "-" + runtime.GOARCH + "/frpc-mesh",
		Mode: 0o755, Size: int64(len(script))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

type upgradeFixture struct {
	u       *Upgrader
	stop    *fakeStop
	bin     string
	ability Capability
	dlURL   string
	sumURL  string
}

func newUpgradeFixture(t *testing.T, body []byte, sumOverride string) *upgradeFixture {
	t.Helper()
	name := assetNameFor("1.2.3")
	sum := sha256.Sum256(body)
	sums := fmt.Sprintf("%x  %s\n", sum, name)
	if sumOverride != "" {
		sums = sumOverride
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, name):
			w.Write(body)
		case strings.HasSuffix(r.URL.Path, checksums):
			fmt.Fprint(w, sums)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	bin := filepath.Join(t.TempDir(), "frpc-mesh")
	if err := os.WriteFile(bin, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	stop := &fakeStop{}
	c := New(newStore(t), "1.0.0")
	u := NewUpgrader(c, stop, t.TempDir())
	u.client = srv.Client()
	// 测试用 httptest 地址不在生产白名单内：放开校验并禁用镜像回退，保证不外连
	u.hostCheck = func(string) error { return nil }
	u.mirrorPrefix = ""
	return &upgradeFixture{
		u: u, stop: stop, bin: bin, dlURL: srv.URL + "/" + name, sumURL: srv.URL + "/" + checksums,
		ability: Capability{CanUpgrade: true, Mode: "binary", AssetName: name, BinaryPath: bin, HasChecksum: true},
	}
}

func (f *upgradeFixture) run(from, to string) {
	f.u.run(f.ability, f.dlURL, f.sumURL, from, to)
}

func TestUpgradeReplacesBinaryAndRestarts(t *testing.T) {
	var gotArgs []string
	var gotEnv []string
	f := newUpgradeFixture(t, fakePackage(t, "1.2.3"), "")
	f.u.execFunc = func(path string, argv, envv []string) error {
		gotArgs, gotEnv = argv, envv
		return nil
	}
	f.run("1.0.0", "1.2.3")

	data, err := os.ReadFile(f.bin)
	if err != nil {
		t.Fatalf("读取替换后的二进制失败: %v", err)
	}
	if !strings.HasPrefix(string(data), "#!/bin/sh") {
		t.Errorf("二进制未被替换: %q", data)
	}
	if f.stop.n != 1 {
		t.Errorf("应停掉 frpc 子进程一次，实际 %d 次", f.stop.n)
	}
	if len(gotArgs) == 0 {
		t.Error("未触发自重启 exec")
	}
	if !strings.Contains(strings.Join(gotEnv, "\n"), ResumeEnv+"=1") {
		t.Errorf("自重启环境缺少 %s: %v", ResumeEnv, gotEnv)
	}
	if !strings.Contains(strings.Join(gotEnv, "\n"), UpgradedEnv+"=1.0.0") {
		t.Errorf("自重启环境缺少升级来源: %v", gotEnv)
	}
	if st := f.u.Status(); st.State != "restarting" || st.To != "1.2.3" {
		t.Errorf("期望状态 restarting/1.2.3，实际 %+v", st)
	}
	// 工作目录应已清理
	if _, err := os.Stat(filepath.Join(f.u.dataDir, "update")); !os.IsNotExist(err) {
		t.Error("升级临时目录未清理")
	}
}

func TestUpgradeAbortsWithChecksumMismatch(t *testing.T) {
	f := newUpgradeFixture(t, fakePackage(t, "1.2.3"), "0000000000000000000000000000000000000000000000000000000000000000  "+assetNameFor("1.2.3")+"\n")
	var execed bool
	f.u.execFunc = func(string, []string, []string) error { execed = true; return nil }

	f.run("1.0.0", "1.2.3")

	if execed {
		t.Error("校验不通过却执行了替换/重启")
	}
	if f.stop.n != 0 {
		t.Error("校验不通过却停掉了 frpc 子进程")
	}
	if data, _ := os.ReadFile(f.bin); string(data) != "old-binary" {
		t.Errorf("原二进制被改动: %q", data)
	}
	if st := f.u.Status(); st.State != "error" || !strings.Contains(st.Message, "校验不匹配") {
		t.Errorf("期望 error/校验不匹配，实际 %+v", st)
	}
}

func TestUpgradeAbortsWhenSelfCheckFails(t *testing.T) {
	// 包内二进制 --version 报告 9.9.9，与目标 1.2.3 不符
	f := newUpgradeFixture(t, fakePackage(t, "9.9.9"), "")
	var execed bool
	f.u.execFunc = func(string, []string, []string) error { execed = true; return nil }

	f.run("1.0.0", "1.2.3")

	if execed {
		t.Error("自检失败却执行了替换")
	}
	if data, _ := os.ReadFile(f.bin); string(data) != "old-binary" {
		t.Errorf("原二进制被改动: %q", data)
	}
	if st := f.u.Status(); st.State != "error" || !strings.Contains(st.Message, "已放弃升级") {
		t.Errorf("期望 error/已放弃升级，实际 %+v", st)
	}
}

func TestUpgradeKeepsOriginalWhenSwapFails(t *testing.T) {
	f := newUpgradeFixture(t, fakePackage(t, "1.2.3"), "")
	// 二进制所在目录只读：备份/替换都会失败，原二进制必须保持可用
	if err := os.Chmod(filepath.Dir(f.bin), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(f.bin), 0o700) })

	var execed bool
	f.u.execFunc = func(string, []string, []string) error { execed = true; return nil }
	f.run("1.0.0", "1.2.3")

	if execed || f.stop.n != 0 {
		t.Errorf("替换失败却继续重启: execed=%v stop=%d", execed, f.stop.n)
	}
	if data, err := os.ReadFile(f.bin); err != nil || string(data) != "old-binary" {
		t.Errorf("原二进制未保留: %q err=%v", data, err)
	}
	if st := f.u.Status(); st.State != "error" {
		t.Errorf("期望 error，实际 %+v", st)
	}
}

func TestExtractBinaryWritesOnlyToDest(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "pkg.tar.gz")
	var raw bytes.Buffer
	zw := gzip.NewWriter(&raw)
	tw := tar.NewWriter(zw)
	// 条目名带目录前缀，落盘位置只由 dest 决定
	for _, entry := range []string{"frpc-mesh-v1.2.3-linux-arm64/frpc-mesh", "docs/frpc-mesh"} {
		hdr := &tar.Header{Name: entry, Mode: 0o644, Size: 2}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("x\n")); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	zw.Close()
	if err := os.WriteFile(archive, raw.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	if err := extractBinary(archive, dest); err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "out,pkg.tar.gz" {
		t.Errorf("解包写出了预期外的文件: %v", names)
	}
}

func TestAllowedHost(t *testing.T) {
	for _, ok := range []string{
		"https://github.com/busfly/frpc-mesh/releases/download/v1/x.tar.gz",
		"https://objects.githubusercontent.com/x",
		"https://ghfast.top/https://github.com/x",
	} {
		if err := allowedHost(ok); err != nil {
			t.Errorf("%s 应允许: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://github.com/x",
		"https://evil.com/x",
		"https://127.0.0.1:8080/x",
		"https://ghfast.top.evil.com/x",
	} {
		if err := allowedHost(bad); err == nil {
			t.Errorf("%s 应拒绝", bad)
		}
	}
}

func TestGetRejectsDisallowedHostBeforeMirrorFallback(t *testing.T) {
	var probed []string
	u := NewUpgrader(New(newStore(t), "1.0.0"), &fakeStop{}, t.TempDir())
	u.client = &http.Client{Transport: &probeTransport{seen: &probed}}

	// 白名单外的地址必须在发出任何请求（含拼到镜像再请求）之前被拒绝
	for _, bad := range []string{"http://127.0.0.1:9/x.tar.gz", "https://evil.example.com/x.tar.gz"} {
		if _, err := u.get(bad); err == nil {
			t.Fatalf("%s 应被拒绝", bad)
		}
	}
	if len(probed) != 0 {
		t.Errorf("白名单外地址不应产生外发请求，实际 %v", probed)
	}
}

type probeTransport struct{ seen *[]string }

func (p *probeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	*p.seen = append(*p.seen, r.URL.String())
	return nil, fmt.Errorf("不该发出请求")
}

func TestRecordAndLastUpgrade(t *testing.T) {
	st := newStore(t)
	c := New(st, "1.0.1")
	c.RecordUpgrade("1.0.0", "1.0.1")
	if got := c.LastUpgrade(); got != "1.0.0→1.0.1" {
		t.Errorf("升级记录 %q", got)
	}
	c.RecordUpgrade("", "1.0.1")
	c.RecordUpgrade("1.0.1", "1.0.1")
	if got := c.LastUpgrade(); got != "1.0.0→1.0.1" {
		t.Errorf("无变化/空来源不应覆盖记录，实际 %q", got)
	}
}
