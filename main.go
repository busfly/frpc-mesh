package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
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
	"frpc-mesh/internal/runner"
	"frpc-mesh/internal/store"
)

//go:embed all:web/dist
var webDist embed.FS

const Version = "1.0.0"

func main() {
	var httpAddr, dataDir string
	var resetPassword bool
	flag.StringVar(&httpAddr, "http", "0.0.0.0:18080", "HTTP 监听地址")
	flag.StringVar(&dataDir, "data", "", "数据目录（默认 $FRPC_MESH_DATA 或 ./data）")
	flag.BoolVar(&resetPassword, "reset-password", false, "将 admin 密码重置为 admin123 后退出（配合 docker exec 使用）")
	flag.Parse()

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
	srv := &api.Server{
		Store:   st,
		Mgr:     mgr,
		Auth:    auth.NewService(secret),
		Login:   auth.NewLimiter(),
		Version: Version,
	}
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	srv.WebFS = webFS

	if st.GetSettings().AutoStart {
		go mgr.StartEnabled()
	}

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
	log.Println("正在停止所有 frpc 实例...")
	mgr.StopAll()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
