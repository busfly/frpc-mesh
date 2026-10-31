// 自升级：仅适用于直接运行二进制的场景（NAS / 软路由 ./start.sh）。
// 容器内不自动替换二进制（重建即失效），只给手动升级指引。
package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// ResumeEnv 由自升级后的新进程读取：即使 auto_start 关闭也拉起已启用服务器
	ResumeEnv = "FRPC_MESH_RESUME"
	// UpgradedEnv 传递升级前的版本号，新进程据此记录升级结果
	UpgradedEnv = "FRPC_MESH_UPGRADED_FROM"
	checksums   = "sha256sums.txt"
	wOK         = 0x2
)

// Stoppable 由 runner.Manager 实现，升级前需要优雅停掉所有 frpc 子进程
type Stoppable interface {
	StopAll()
}

// Capability 描述当前环境能否自动升级
type Capability struct {
	CanUpgrade  bool   `json:"can_upgrade"`
	Mode        string `json:"mode"` // binary | container | unsupported
	Reason      string `json:"reason"`
	AssetName   string `json:"asset_name"`
	BinaryPath  string `json:"binary_path,omitempty"`
	HasChecksum bool   `json:"has_checksum"`
}

type UpgradeState struct {
	State   string `json:"state"` // idle | downloading | verifying | swapping | restarting | done | error
	Message string `json:"message"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type Upgrader struct {
	mu      sync.Mutex
	job     UpgradeState
	c       *Checker
	stop    Stoppable
	dataDir string
	client  *http.Client
	// hostCheck / mirrorPrefix / execFunc 为可注入项，测试中替换以隔离真实网络与进程
	hostCheck    func(string) error
	mirrorPrefix string
	execFunc     func(path string, argv, envv []string) error
}

func NewUpgrader(c *Checker, stop Stoppable, dataDir string) *Upgrader {
	return &Upgrader{
		c: c, stop: stop, dataDir: dataDir,
		client:       &http.Client{Timeout: 10 * time.Minute},
		hostCheck:    allowedHost,
		mirrorPrefix: mirror,
		execFunc:     syscall.Exec,
	}
}

func assetNameFor(latestVer string) string {
	return fmt.Sprintf("frpc-mesh-v%s-%s-%s.tar.gz", latestVer, runtime.GOOS, runtime.GOARCH)
}

// CapabilityFor 判定环境：容器 / 二进制、二进制是否可替换、目标平台是否有包与校验和
func (u *Upgrader) CapabilityFor(latestVer string, assets []Asset) Capability {
	asset := assetNameFor(latestVer)
	cap := Capability{Mode: "binary", AssetName: asset}
	if latestVer == "" {
		cap.Mode = "unsupported"
		cap.Reason = "尚未检查到可用版本"
		return cap
	}
	var found, hasSum bool
	for _, a := range assets {
		if a.Name == asset {
			found = true
		}
		if a.Name == checksums {
			hasSum = true
		}
	}
	cap.HasChecksum = hasSum
	if inContainer() {
		cap.Mode = "container"
		cap.Reason = "检测到运行在容器中：容器内替换的二进制会在镜像重建时丢失，请改用新的镜像包再重建容器"
		return cap
	}
	if !found {
		cap.Mode = "unsupported"
		cap.Reason = fmt.Sprintf("该版本未提供当前平台（%s/%s）的安装包，请手动下载", runtime.GOOS, runtime.GOARCH)
		return cap
	}
	if !hasSum {
		cap.Mode = "unsupported"
		cap.Reason = fmt.Sprintf("该版本缺少 %s 校验和文件，为避免替换成损坏的二进制，请手动下载", checksums)
		return cap
	}
	bin, err := os.Executable()
	if err != nil {
		cap.Mode = "unsupported"
		cap.Reason = fmt.Sprintf("无法定位当前二进制: %v", err)
		return cap
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	cap.BinaryPath = bin
	if !writable(bin) {
		cap.Mode = "unsupported"
		cap.Reason = fmt.Sprintf("当前二进制不可写（%s），请用有权限的账号手动替换", filepath.Dir(bin))
		return cap
	}
	cap.CanUpgrade = true
	cap.Reason = "可直接升级：替换二进制后进程自重启，隧道会中断数秒"
	return cap
}

// Start 启动一次后台升级；同一时刻只允许一个任务
func (u *Upgrader) Start(to string) error {
	if !versionRe.MatchString(to) {
		return fmt.Errorf("非法的目标版本号: %s", to)
	}
	u.mu.Lock()
	cur := u.job
	u.mu.Unlock()
	if runningState(cur.State) {
		return fmt.Errorf("升级正在进行中（%s）", cur.State)
	}

	inf := u.c.Status()
	cap := u.CapabilityFor(inf.Latest, inf.Assets)
	if !cap.CanUpgrade {
		return fmt.Errorf("%s", cap.Reason)
	}
	if inf.Latest != to {
		return fmt.Errorf("目标版本 v%s 与最新已发布版本 v%s 不一致", to, inf.Latest)
	}
	var dlURL, sumURL string
	for _, a := range inf.Assets {
		switch a.Name {
		case cap.AssetName:
			dlURL = a.URL
		case checksums:
			sumURL = a.URL
		}
	}
	if dlURL == "" || sumURL == "" {
		return fmt.Errorf("未找到 v%s 的下载或校验和地址，请稍后重新检查", to)
	}

	from := inf.Current
	u.set(UpgradeState{State: "downloading", Message: "正在下载安装包", From: from, To: to})
	go u.run(cap, dlURL, sumURL, from, to)
	return nil
}

// Status 返回当前/最近一次升级任务状态
func (u *Upgrader) Status() UpgradeState {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.State == "" {
		return UpgradeState{State: "idle"}
	}
	return u.job
}

func (u *Upgrader) set(st UpgradeState) {
	u.mu.Lock()
	u.job = st
	u.mu.Unlock()
}

func (u *Upgrader) fail(from, to string, err error) {
	u.set(UpgradeState{State: "error", Message: err.Error(), From: from, To: to})
}

func (u *Upgrader) run(cap Capability, dlURL, sumURL, from, to string) {
	workDir := filepath.Join(u.dataDir, "update")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		u.fail(from, to, err)
		return
	}
	defer os.RemoveAll(workDir)

	archive := filepath.Join(workDir, cap.AssetName)
	if err := u.download(dlURL, archive); err != nil {
		u.fail(from, to, err)
		return
	}

	u.set(UpgradeState{State: "verifying", Message: "正在校验下载文件", From: from, To: to})
	want, err := u.expectedSum(sumURL, cap.AssetName)
	if err != nil {
		u.fail(from, to, err)
		return
	}
	got, err := fileSum(archive)
	if err != nil {
		u.fail(from, to, err)
		return
	}
	if !strings.EqualFold(got, want) {
		u.fail(from, to, fmt.Errorf("sha256 校验不匹配（期望 %s，实际 %s），已放弃升级", want, got))
		return
	}

	newBin := filepath.Join(workDir, "frpc-mesh-new")
	if err := extractBinary(archive, newBin); err != nil {
		u.fail(from, to, err)
		return
	}
	if err := verifyRuns(newBin, to); err != nil {
		u.fail(from, to, err)
		return
	}

	u.set(UpgradeState{State: "swapping", Message: "正在替换二进制", From: from, To: to})
	// 替换须与目标在同一文件系统，故暂存文件落在二进制所在目录
	staged := cap.BinaryPath + ".new-" + to
	if err := copyFile(newBin, staged, 0o755); err != nil {
		u.fail(from, to, err)
		return
	}
	backup := cap.BinaryPath + ".bak"
	if err := os.Rename(cap.BinaryPath, backup); err != nil {
		os.Remove(staged)
		u.fail(from, to, fmt.Errorf("备份旧二进制失败: %v", err))
		return
	}
	if err := os.Rename(staged, cap.BinaryPath); err != nil {
		os.Remove(staged)
		if rbErr := os.Rename(backup, cap.BinaryPath); rbErr != nil {
			u.fail(from, to, fmt.Errorf("替换失败且回滚失败: %v；原二进制已备份为 %s", err, backup))
			return
		}
		u.fail(from, to, fmt.Errorf("替换失败，已回滚到原二进制: %v", err))
		return
	}
	// 保留 .bak 以便手工回滚，不做静默删除
	u.set(UpgradeState{State: "restarting", Message: fmt.Sprintf("已替换为 v%s，正在重启服务（隧道将中断数秒）", to), From: from, To: to})
	u.stop.StopAll()

	env := append(os.Environ(), ResumeEnv+"=1", UpgradedEnv+"="+from)
	if err := u.execFunc(cap.BinaryPath, os.Args, env); err != nil {
		u.set(UpgradeState{State: "error", To: to, From: from,
			Message: fmt.Sprintf("重启失败，请用原命令手动启动 v%s（二进制已替换成功）: %v", to, err)})
	}
}

func (u *Upgrader) download(urlStr, dest string) error {
	data, err := u.get(urlStr)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// get 只允许 GitHub 与既定镜像域名，避免 Release 数据被改写后指向任意主机
func (u *Upgrader) get(urlStr string) ([]byte, error) {
	// 先校验原始地址：不在允许列表就不做任何回退，防止拼到镜像后绕过校验
	if err := u.hostCheck(urlStr); err != nil {
		return nil, err
	}
	data, err := u.getRaw(urlStr)
	if err == nil {
		return data, nil
	}
	if u.mirrorPrefix == "" {
		return nil, err
	}
	alt := u.mirrorPrefix + urlStr
	if u.hostCheck(alt) != nil {
		return nil, err
	}
	if altData, altErr := u.getRaw(alt); altErr == nil {
		return altData, nil
	}
	return nil, err
}

func (u *Upgrader) getRaw(urlStr string) ([]byte, error) {
	if err := u.hostCheck(urlStr); err != nil {
		return nil, err
	}
	resp, err := u.client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载返回状态 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return nil, fmt.Errorf("读取下载内容失败: %v", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("下载内容为空")
	}
	return body, nil
}

// expectedSum 从 sha256sums.txt 中取出目标文件的校验和
func (u *Upgrader) expectedSum(sumURL, asset string) (string, error) {
	body, err := u.get(sumURL)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%s 中未找到 %s 的校验和", checksums, asset)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary 从发布包中取出 frpc-mesh 主程序；只按文件名匹配，写到固定目标，
// 不使用压缩包内的路径
func extractBinary(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("安装包中未找到 frpc-mesh 主程序")
		}
		if err != nil {
			return fmt.Errorf("读取安装包失败: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "frpc-mesh" {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}

// verifyRuns 用新二进制的 --version 自检，避免替换成不匹配的程序
func verifyRuns(bin, want string) error {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("新版本二进制自检失败: %v（输出 %q）", err, strings.TrimSpace(string(out)))
	}
	got := strings.TrimSpace(string(out))
	if got != "v"+want && got != want {
		return fmt.Errorf("新版本二进制报告版本 %q，期望 v%s，已放弃升级", got, want)
	}
	return nil
}

func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	return out.Close()
}

func writable(path string) bool {
	if err := syscall.Access(path, wOK); err != nil {
		return false
	}
	return syscall.Access(filepath.Dir(path), wOK) == nil
}

// inContainer 判定是否运行在容器内（测试可替换该变量）
var inContainer = defaultInContainer

func defaultInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(data)
	for _, marker := range []string{"docker", "containerd", "kubepods", "libpod", "podman", "lxc"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// allowedHost 限制升级下载只能命中 GitHub 资源或既定镜像
func allowedHost(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("不安全的下载地址: %s", urlStr)
	}
	host := strings.ToLower(u.Host)
	allowed := host == "github.com" || host == "api.github.com" ||
		host == "objects.githubusercontent.com" || host == "raw.githubusercontent.com" ||
		host == "ghfast.top" || strings.HasSuffix(host, ".ghfast.top")
	if !allowed {
		return fmt.Errorf("下载地址域名不在允许列表: %s", host)
	}
	return nil
}

func runningState(s string) bool {
	switch s {
	case "downloading", "verifying", "swapping", "restarting":
		return true
	}
	return false
}

// RecordUpgrade 记录新进程启动时的升级结果，便于界面给出确定反馈
func (c *Checker) RecordUpgrade(from, to string) {
	if from == "" || from == to {
		return
	}
	b, _ := json.Marshal(map[string]string{"from": from, "to": to, "at": time.Now().Format(time.RFC3339)})
	c.store.SetSetting("last_upgrade", string(b))
}

// LastUpgrade 返回上次自升级记录（"1.0.0→1.0.1"），无记录则为空
func (c *Checker) LastUpgrade() string {
	raw := c.store.GetSetting("last_upgrade", "")
	if raw == "" {
		return ""
	}
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	return fmt.Sprintf("%s→%s", m["from"], m["to"])
}
