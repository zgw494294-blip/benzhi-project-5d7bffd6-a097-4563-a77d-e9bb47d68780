package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"groundwater-release/internal/httpapi"
	"groundwater-release/internal/service"
	"groundwater-release/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "服务退出:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config{}
	flag.StringVar(&cfg.address, "addr", defaultAddress(), "回环监听地址")
	flag.StringVar(&cfg.databasePath, "db", "groundwater-release.db", "SQLite 数据库路径")
	flag.BoolVar(&cfg.selfcheck, "selfcheck", false, "执行完整 HTTP 自检后退出")
	flag.DurationVar(&cfg.selfcheckTimeout, "selfcheck-timeout", 15*time.Second, "自检总超时")
	flag.Parse()
	if err := validateAddress(cfg.address); err != nil {
		return err
	}
	cleanup := func() {}
	if cfg.selfcheck {
		file, err := os.CreateTemp("", "groundwater-release-selfcheck-*.db")
		if err != nil {
			return err
		}
		cfg.databasePath = file.Name()
		if err = file.Close(); err != nil {
			return err
		}
		cleanup = func() { _ = os.Remove(cfg.databasePath) }
	}
	defer cleanup()
	repo, err := store.Open(cfg.databasePath)
	if err != nil {
		return fmt.Errorf("打开 SQLite: %w", err)
	}
	defer repo.Close()
	app := service.New(repo)
	api := httpapi.New(app)
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.address, err)
	}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	if cfg.selfcheck {
		return runBoundedSelfcheck(server, listener, serveDone, cfg.selfcheckTimeout)
	}
	fmt.Printf("井澈质量放行服务正在监听 http://%s\n", listener.Addr().String())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case serveErr := <-serveDone:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = server.Shutdown(ctx); err != nil {
		return err
	}
	serveErr := <-serveDone
	if !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func runBoundedSelfcheck(server *http.Server, listener net.Listener, serveDone <-chan error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	checkErr := runSelfcheck(ctx, "http://"+listener.Addr().String())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	shutdownErr := server.Shutdown(shutdownCtx)
	shutdownCancel()
	serveErr := <-serveDone
	if checkErr != nil {
		return checkErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	if !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	fmt.Println("selfcheck 通过：已完成批次创建、采样质控、整改复核、批准、冻结和凭据签发")
	return nil
}
