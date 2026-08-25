# BENZHI_README

基于 Go 实现的井澈地下水采样质量放行服务 HTTP API 项目，一款后端服务，已完整实现井澈地下水采样质量放行服务，提供 SQLite 持久化、乐观并发与幂等写入、确定性质量检查、异常整改复核、技术批准、数据冻结、摘要审计链和不可变放行凭据，并包含真实回环 HTTP 自检。

## 项目说明
- 项目：benzhi-project-5d7bffd6-a097-4563-a77d-e9bb47d68780
- 项目用途：已完整实现井澈地下水采样质量放行服务，提供 SQLite 持久化、乐观并发与幂等写入、确定性质量检查、异常整改复核、技术批准、数据冻结、摘要审计链和不可变放行凭据，并包含真实回环 HTTP 自检。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=15s
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-5d7bffd6-a097-4563-a77d-e9bb47d68780-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-5d7bffd6-a097-4563-a77d-e9bb47d68780-arm64 linux/arm64
docker run -it benzhi-project-5d7bffd6-a097-4563-a77d-e9bb47d68780-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=15s`
