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
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18900", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	store, err := account.Open(*dataDir)
	if err != nil {
		log.Fatalf("加载账号失败: %v", err)
	}
	secret, err := auth.LoadOrCreateSecret(*dataDir)
	if err != nil {
		log.Fatalf("加载会话密钥失败: %v", err)
	}
	adminHash, err := auth.LoadOrCreateAdmin(*dataDir)
	if err != nil {
		log.Fatalf("加载管理员口令失败: %v", err)
	}

	srv := server.New(server.Config{
		DataDir:   *dataDir,
		Store:     store,
		AdminHash: adminHash,
		Signer:    auth.NewSigner(secret),
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
