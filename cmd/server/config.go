package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	address          string
	databasePath     string
	selfcheck        bool
	selfcheckTimeout time.Duration
}

func defaultAddress() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return "127.0.0.1:19081"
	}
	return net.JoinHostPort("127.0.0.1", port)
}

func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("监听地址必须采用 host:port 格式: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" || host == "*" {
		return fmt.Errorf("监听地址不允许空主机或通配主机")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("监听地址必须使用回环主机")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("监听端口必须位于 1 到 65535")
	}
	return nil
}
