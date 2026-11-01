package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"frpc-mesh/internal/api"
	"frpc-mesh/internal/auth"
	"frpc-mesh/internal/logging"
	"frpc-mesh/internal/runner"
	"frpc-mesh/internal/store"
	"frpc-mesh/internal/update"
)

//go:embed all:web/dist
var webDist embed.FS

// Version 构建期由 make release 注入（-X main.Version=1.1.0，不带 v 前缀），开发构建保留默认值
var Version = "1.1.0"

func main() {
	var httpAddr, dataDir, logDir string
	var resetPassword, showVersion bool
	flag.StringVar(&httpAddr, "http", "0.0.0.0:18080", "HTTP 监听地址")
	flag.StringVar(&dataDir, "data", "", "数据目录（默认 $FRPC_MESH_DATA 或 ./data）")
	flag.StringVar(&logDir, "log-dir", "", "运行日志同时落盘到该目录（默认取 $FRPC_MESH_LOG_DIR；两者都为空则只写标准输出）")
	flag.BoolVar(&resetPassword, "reset-password", false, "将 admin 密码重置为 admin123 后退出（配合 docker exec 使用）")
	flag.BoolVar(&showVersion, "version", false, "输出版本号后退出")
	flag.Parse()

	if showVersion {
		fmt.Println(Version)
		return
	}

	// 日志落盘：不设参数时与旧版完全一致（只写 stdout）。目录建不出来只警告——
	// "日志写不了"不该让面板起不来
	if logDir == "" {
		logDir = os.Getenv("FRPC_MESH_LOG_DIR")
	}
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			log.Printf("警告: 日志目录 %s 不可用（%v），本次运行日志只写标准输出", logDir, err)
		} else {
			log.SetOutput(io.MultiWriter(os.Stdout, logging.NewRotatingWriter(logDir, "frpc-mesh.log", 10<<20, 5)))
			log.Printf("运行日志同时写入 %s/frpc-mesh.log（单文件 10MB，最多保留 5 份）", logDir)
		}
	}

	if dataDir == "" {
		dataDir = os.Getenv("FRPC_MESH_DATA")
	}
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录: %v", err)
	}

	st, err := store.Open(filepath.Join(dataDir, "frpc-mesh.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	// 首个用户种子 + JWT 密钥
	if st.CountUsers() == 0 {
		h, err := auth.HashPassword("admin123")
		if err != nil {
			log.Fatalf("初始化用户失败: %v", err)
		}
		st.UpsertUser("admin", h)
		log.Println("已创建默认账号 admin / admin123，请登录后尽快修改密码")
	}
	if resetPassword {
		h, err := auth.HashPassword("admin123")
		if err != nil {
			log.Fatalf("重置密码失败: %v", err)
		}
		if err := st.UpsertUser("admin", h); err != nil {
			log.Fatalf("重置密码失败: %v", err)
		}
		log.Println("admin 密码已重置为 admin123，请登录后在「设置-修改密码」自行修改")
		return
	}
	secret := st.GetSetting("jwt_secret", "")
	if secret == "" {
		buf := make([]byte, 32)
		rand.Read(buf)
		secret = hex.EncodeToString(buf)
		st.SetSetting("jwt_secret", secret)
	}

	mgr := runner.NewManager(st, dataDir)
	mgr.CleanTempFiles()
	checker := update.New(st, Version)
	upgrader := update.NewUpgrader(checker, mgr, dataDir)
	srv := &api.Server{
		Store:   st,
		Mgr:     mgr,
		Auth:    auth.NewService(secret),
		Login:   auth.NewLimiter(),
		Update:  checker,
		Upgrade: upgrader,
		Version: Version,
	}
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	srv.WebFS = webFS

	// 自升级重启后的新进程：记录升级结果，并保证已启用服务器被拉回运行
	if from := os.Getenv(update.UpgradedEnv); from != "" {
		checker.RecordUpgrade(from, Version)
		log.Printf("已完成自升级: %s → %s", from, Version)
	}
	if st.GetSettings().AutoStart || os.Getenv(update.ResumeEnv) == "1" {
		go mgr.StartEnabled()
	}

	stopChecks := make(chan struct{})
	go checker.Loop(stopChecks)

	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      srv.Routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	go func() {
		log.Printf("frpc-mesh v%s 已启动: http://%s (数据目录: %s)", Version, httpAddr, dataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务失败: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stopChecks)
	log.Println("正在停止所有 frpc 实例...")
	mgr.StopAll()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
