package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AaronChou313/open-remote-net/agent/internal/api"
	"github.com/AaronChou313/open-remote-net/agent/internal/forward"
	"github.com/AaronChou313/open-remote-net/agent/internal/launcher"
	"github.com/AaronChou313/open-remote-net/agent/internal/mesh"
	"github.com/AaronChou313/open-remote-net/agent/internal/state"
)

var version = "0.1.0"

func main() {
	path := flag.String("config", state.DefaultPath(), "配置文件")
	auth := flag.String("authkey", os.Getenv("TS_AUTHKEY"), "首次入网密钥，也可通过 TS_AUTHKEY 环境变量提供")
	server := flag.String("server", "", "组网控制服务")
	debug := flag.Bool("debug", false, "详细组网日志")
	openPage := flag.Bool("open", false, "启动后打开本地页面")
	flag.Parse()
	cfg, token, err := state.Load(*path)
	if err != nil {
		log.Fatal(err)
	}
	if flag.Arg(0) == "open" {
		if err = launcher.OpenURL("http://" + cfg.APIAddr + "/#token=" + token); err != nil {
			log.Fatal(err)
		}
		return
	}
	if flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}
	if *server != "" {
		cfg.ControlURL = *server
	}
	node := mesh.New(mesh.Config{Hostname: cfg.Hostname, ControlURL: cfg.ControlURL, StateDir: cfg.StateDir, AuthKey: *auth, Debug: *debug, Logf: log.Printf})
	if err = node.Start(); err != nil {
		log.Fatal(err)
	}
	defer node.Close()
	forwards := forward.New(node)
	defer forwards.Close()
	handler := &api.Server{Node: node, Forwards: forwards, Token: token, Config: cfg, ConfigPath: *path, Version: version}
	httpServer := &http.Server{Addr: cfg.APIAddr, Handler: handler.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go handler.CloudLoop(ctx)
	go func() {
		if err := node.Wait(ctx); err != nil {
			log.Printf("组网等待中，请在本地页完成绑定: %v", err)
			return
		}
		for port, target := range cfg.Publish {
			if err := node.Publish(port, target); err != nil {
				log.Printf("发布端口 %d 失败: %v", port, err)
			}
		}
	}()
	go func() {
		<-ctx.Done()
		end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(end)
	}()
	if *openPage {
		go launcher.OpenURL("http://" + cfg.APIAddr + "/#token=" + token)
	}
	log.Printf("ORN %s · http://%s · 配置 %s", version, cfg.APIAddr, filepath.Clean(*path))
	if err = httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
