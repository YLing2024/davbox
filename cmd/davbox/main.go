// Command davbox 启动一个按账号隔离的 WebDAV 服务，并内置 admin / client 页面。
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/YLing2024/davbox/internal/account"
	"github.com/YLing2024/davbox/internal/auth"
	"github.com/YLing2024/davbox/internal/server"
	"github.com/YLing2024/davbox/internal/settings"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18900", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录")
	flag.Parse()

	authMode, err := auth.ParseMode(os.Getenv("AUTH_MODE"))
	if err != nil {
		log.Fatalf("%v", err)
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	store, err := account.Open(*dataDir)
	if err != nil {
		log.Fatalf("加载账号失败: %v", err)
	}
	// settings.json 优先；没有该键时用 CORS_ORIGINS 作为初始默认。
	sett, err := settings.Open(*dataDir, os.Getenv("CORS_ORIGINS"))
	if err != nil {
		log.Fatalf("加载设置失败: %v", err)
	}
	if n := len(sett.Current().CORSOrigins); n > 0 {
		log.Printf("跨域白名单已启用（%d 个来源，可在管理页面修改后即时生效）", n)
	}
	secret, err := auth.LoadOrCreateSecret(*dataDir)
	if err != nil {
		log.Fatalf("加载会话密钥失败: %v", err)
	}

	// builtin 模式加载（必要时生成）自带管理员口令；sso 模式不碰自带账号体系。
	var adminHash []byte
	if authMode.IsSSO() {
		log.Printf("管理端认证: SSO（信任 %s，请确保仅在网关后暴露）", auth.HeaderAuthUser)
	} else {
		adminHash, err = auth.LoadOrCreateAdmin(*dataDir)
		if err != nil {
			log.Fatalf("加载管理员口令失败: %v", err)
		}
	}

	srv := server.New(server.Config{
		DataDir:       *dataDir,
		Store:         store,
		AdminHash:     adminHash,
		Signer:        auth.NewSigner(secret),
		AuthMode:      authMode,
		Settings:      sett,
		Preconditions: os.Getenv("WEBDAV_PRECONDITIONS"),
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	log.Printf("davbox 已启动，监听 http://%s，数据目录 %s", *addr, *dataDir)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务退出: %v", err)
	}
}
