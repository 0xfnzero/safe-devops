.DEFAULT_GOAL := help
SHELL := /bin/bash
.NOTPARALLEL:
PYTHON ?= python3
VERSION ?= 0.4.1
OS ?= linux
ARCH ?= amd64
DESKTOP_OS ?= native
DESKTOP_ARCH ?= native
CONFIG_DIR ?= .safeops/production
DATABASE ?= sqlite
WAIT_TIMEOUT ?= 240
LOG_TAIL ?= 100
DRY_RUN ?= 0
START ?= 0
UPGRADE ?= 0
export VERSION OS ARCH DESKTOP_OS DESKTOP_ARCH CONFIG_DIR DATABASE WAIT_TIMEOUT LOG_TAIL
export WORKSPACE PROJECT BROKER_URL WEB_URL APPROVER_PUBLIC_KEY_FILE DEVICE_PUBLIC_KEY_FILE DIAGNOSTIC_ROOT
export SERVICE BACKUP_FILE COMPONENT BINARY SHA256 CONFIG ENV_FILE WEB_BUNDLE DRY_RUN START UPGRADE
export SAFE_DEVOPS_BIN WEB_PORT BROKER_PORT DEV_DIR
ifeq ($(origin VERSION),command line)
export SAFEOPS_VERSION := $(VERSION)
endif

.PHONY: help setup setup-desktop doctor dev demo dev-backend dev-web dev-desktop desktop build build-all build-web test test-web test-desktop test-scripts check smoke smoke-tls desktop-check package-server package-servers package-web package-source package-desktop package-macos package-windows package-linux release docker-build deploy-init deploy-check deploy-up deploy-status deploy-logs deploy-health deploy-restart deploy-stop deploy-down deploy-backup deploy-restore deploy-upgrade deploy-policy-apply install install-check service-start service-stop service-status service-logs

##@ 入门与开发
help: ## 显示所有命令和说明（直接 make 等同于 make help）
	@awk 'BEGIN {print "safe-devops · make <命令> [变量=值]"} /^##@/ {printf "\n%s\n", substr($$0,5)} /^[a-zA-Z0-9_-]+:.*## / {split($$0,a,":"); sub(/^.*## /,""); printf "  %-22s %s\n", a[1], $$0}' Makefile
	@printf '\n参数与示例见 docs/commands.md；生产配置默认 .safeops/production，数据库默认 SQLite。\n'

setup: ## 安装 Go / Web 依赖并构建本机 CLI
	cd server && go mod download
	cd web && npm ci
	$(MAKE) build

setup-desktop: ## 安装 Electron 客户端依赖
	cd desktop && npm ci

doctor: ## 检查本机开发、打包与部署工具是否可用
	@$(PYTHON) scripts/tasks.py doctor

dev: build ## 启动完整本机演示环境；Ctrl+C 停止本次启动的所有进程
	@$(PYTHON) scripts/dev.py

demo: dev ## dev 的兼容别名

dev-backend: build ## 仅启动演示 Broker、两个连接器和模拟 AI 审核器
	@$(PYTHON) scripts/dev.py --backend-only

dev-web: ## 单独启动 Web 热更新；连接已运行的本机演示 Broker
	@$(PYTHON) scripts/tasks.py dev-web

dev-desktop: build ## 启动完整演示环境及 Electron（先 setup-desktop）
	@$(PYTHON) scripts/dev.py --desktop

desktop: ## 仅启动 Electron 开发客户端（Web 需已运行）
	cd desktop && npm run dev

##@ 构建与检查
build: ## 构建本机 Go CLI 到 bin/safe-devops
	@$(PYTHON) scripts/tasks.py build

build-all: ## 交叉构建 Linux / macOS 的 amd64、arm64 Go CLI
	@$(PYTHON) scripts/package.py binaries

build-web: ## 构建当前平台的 Next.js 生产版本
	cd web && npm run build

test: ## 运行 Go race 测试及 go vet
	cd server && go test -race ./... && go vet ./...

test-web: ## 检查 Web 类型与 ESLint
	cd web && npm run typecheck && npm run lint

test-desktop: ## 运行 Electron 安全策略及设备签名测试
	cd desktop && npm test

test-scripts: ## 验证命令入口、打包隔离与部署脚本边界
	@$(PYTHON) -m unittest discover -s scripts/tests -v

check: test test-web test-desktop test-scripts build-web ## 完整本地检查和 Web 生产构建

smoke: build ## 验证已运行的本机演示业务流程（需先 make dev）
	$(PYTHON) scripts/smoke.py
	$(PYTHON) scripts/smoke-terminal.py

smoke-tls: build ## 在隔离环境验证生产 TLS、设备签名及审批闭环
	$(PYTHON) scripts/test-local-tls.py

desktop-check: test-desktop ## 测试并打包本平台未签名 Electron 应用目录
	cd desktop && npm run pack

##@ 分发打包（输出到 dist/，不覆盖已有版本）
package-server: ## 打包 Go CLI + systemd 工具；OS=linux ARCH=amd64
	@$(PYTHON) scripts/package.py server

package-servers: ## 打包 Linux / macOS 双架构共四个 Go CLI 包
	@$(PYTHON) scripts/package.py servers

package-web: ## 打包本平台 Next.js standalone、静态资源与校验清单
	@$(PYTHON) scripts/package.py web

package-source: ## 打包源码与部署文件；排除凭据、依赖、演示数据
	@$(PYTHON) scripts/package.py source

package-desktop: ## 打包桌面安装包；DESKTOP_OS=native DESKTOP_ARCH=native
	@$(PYTHON) scripts/package.py desktop

package-macos: ## macOS 上构建 dmg / zip；DESKTOP_ARCH=amd64 或 arm64
	@$(PYTHON) scripts/package.py desktop --desktop-os macos

package-windows: ## Windows 上构建 NSIS exe；DESKTOP_ARCH=amd64 或 arm64
	@$(PYTHON) scripts/package.py desktop --desktop-os windows

package-linux: ## Linux 上构建 AppImage / deb；DESKTOP_ARCH=amd64 或 arm64
	@$(PYTHON) scripts/package.py desktop --desktop-os linux

release: ## 构建四平台 CLI 包、当前平台 Web 包及源码包；VERSION=版本号
	@$(PYTHON) scripts/package.py release

docker-build: ## 构建当前 Docker 平台的 server / web 镜像
	@$(PYTHON) scripts/tasks.py docker-build

##@ Docker Compose 服务器部署（CONFIG_DIR=配置目录）
deploy-init: ## 初始化生产配置；必填域名、团队、项目及两份公钥文件
	@$(PYTHON) scripts/deploy.py init

deploy-check: ## 检查 Compose 与 Broker 配置（镜像需已构建）
	@$(PYTHON) scripts/deploy.py check

deploy-up: ## 构建、启动控制端并等待健康；自动选择 SQLite / PG
	@$(PYTHON) scripts/deploy.py up

deploy-status: ## 查看部署服务状态
	@$(PYTHON) scripts/deploy.py status

deploy-logs: ## 跟踪日志；可指定 SERVICE=broker LOG_TAIL=100
	@$(PYTHON) scripts/deploy.py logs

deploy-health: ## 检查 Broker 数据库就绪及 Web 响应
	@$(PYTHON) scripts/deploy.py health

deploy-restart: ## 重启并等待健康；可指定 SERVICE=web
	@$(PYTHON) scripts/deploy.py restart

deploy-stop: ## 停止服务，保留容器和持久数据
	@$(PYTHON) scripts/deploy.py stop

deploy-down: ## 移除本部署容器与网络，保留数据库卷和配置
	@$(PYTHON) scripts/deploy.py down

deploy-backup: ## 在线备份团队；WORKSPACE=团队，输出到配置目录/backups
	@$(PYTHON) scripts/deploy.py backup

deploy-restore: ## 恢复到空分区；须先停 Broker，BACKUP_FILE=备份文件名
	@$(PYTHON) scripts/deploy.py restore

deploy-upgrade: ## 预构建、校验、备份所有团队，再更新并等待健康
	@$(PYTHON) scripts/deploy.py upgrade

deploy-policy-apply: ## 停 Broker 后迁移策略指纹；WORKSPACE=团队，旧授权须过期
	@$(PYTHON) scripts/deploy.py policy-apply

##@ Linux systemd 安装与维护
install: ## 安装组件；COMPONENT/BINARY/SHA256/CONFIG 等参数见文档
	@$(PYTHON) scripts/deploy.py install

install-check: ## 校验安装输入并打印计划，不修改系统
	@$(PYTHON) scripts/deploy.py install --dry-run

service-start: ## 启用并启动指定 COMPONENT（broker/web/connector/reviewer/change-runner）
	@$(PYTHON) scripts/deploy.py service-start

service-stop: ## 停止指定 COMPONENT 的 systemd 服务
	@$(PYTHON) scripts/deploy.py service-stop

service-status: ## 查看指定 COMPONENT 的 systemd 状态
	@$(PYTHON) scripts/deploy.py service-status

service-logs: ## 跟踪指定 COMPONENT 的 journal 日志
	@$(PYTHON) scripts/deploy.py service-logs
