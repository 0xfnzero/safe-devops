# Make 命令、打包与部署

在仓库根目录执行 `make` 或 `make help`，会按用途列出所有命令及描述，不会自动安装依赖或启动服务。命令说明直接来自 Makefile。`make doctor` 显示工具路径和环境要求。

开发与打包需要 Go 1.26+、Node.js 22.18+（推荐 24）、npm、Python 3.11+、GNU Make 和 Bash。生产 Compose 需要 Docker Engine 与 Compose v2（支持 `up --wait`）；SQLite 为默认数据库。systemd 安装要求 Linux，写系统文件时使用管理员权限。Windows Electron 可直接运行 Python 打包入口；完整开发环境使用 WSL。

## 开发环境

```bash
make setup                 # 安装 Go / Web 依赖，构建 CLI
make dev                   # Broker、两个 Connector、模拟 AI、Web 热更新
# Ctrl+C 关闭本次启动的整个进程组，包括 Next / Electron 子进程

make setup-desktop
make dev-desktop           # 同时启动上述服务和 Electron 启动器
# 已有 Web 服务时，只启动客户端：
make desktop
```

默认 Web 为 `http://127.0.0.1:3000`、Broker 为 `127.0.0.1:8088`，演示数据保存在 `.safeops/v4`。`make demo` 是 `make dev` 的兼容别名。客户端在启动器填写 Web 地址；开发命令不会保存登录 token，也不会改已有客户端设备密钥。

服务启动前检查端口、依赖和演示配置；已有端口占用时直接报错，不杀死其他进程。一个服务异常退出会停止同次启动的其他进程。环境中存在 `SAFEOPS_DATABASE_URL` 时拒绝启动演示，防止误接外部数据库。

也可分开运行：

```bash
# 终端一
make dev-backend
# 终端二
make dev-web
```

需要另一个演示数据目录或端口时：

```bash
make dev DEV_DIR=.safeops/dev-alice BROKER_PORT=18088 WEB_PORT=13000
```

新目录初始化时匹配端口；已有目录端口不一致会报错，不会重写配置。Next.js 同一源码目录只运行一个开发实例；这些参数也可用于避开本机其他软件的端口。

## 检查

```bash
make test                  # Go race + vet
make test-web              # TypeScript + ESLint
make test-desktop          # 设备签名和客户端策略测试
make test-scripts          # 打包 / 部署保护、默认帮助与脚本检查
make check                 # 上述全部 + Web 生产构建
make smoke                 # 依赖默认端口上已运行的 make dev
make smoke-tls             # 隔离临时环境的 TLS / 审批闭环
```

`make smoke` 会创建演示操作和风控记录，仅用于本机演示环境。PostgreSQL 集成测试使用独立 `*_test` 数据库，与生产数据库分开。

## 服务器 / CLI 与 Web 包

```bash
make build                                      # 本机 bin/safe-devops
make build-all VERSION=0.4.1                     # 四平台裸二进制 + SHA256SUMS
make package-server VERSION=0.4.1 OS=linux ARCH=amd64
make package-server VERSION=0.4.1 OS=linux ARCH=arm64
make package-server VERSION=0.4.1 OS=darwin ARCH=arm64
make package-servers VERSION=0.4.1               # 四种组合一次打包
make package-web VERSION=0.4.1                   # 当前构建机 OS / 架构
make package-source VERSION=0.4.1                # 可用于 Docker 构建的源码包
make release VERSION=0.4.1                       # 四个 CLI 包 + 本机 Web + 源码
```

输出目录 `dist/`。CLI 支持 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`；CGO 关闭。**生产服务部署在 Linux**；macOS CLI 用于管理员操作与开发。Windows 使用 Electron 客户端，当前没有 Windows 服务端二进制。

每个 CLI 压缩包包含 `bin/safe-devops`、systemd 模板、`scripts/install.sh` 和 `INSTALL.md`。Web 包包含 `server.js`、运行依赖、`.next/static`、`public`；需要相同 OS / 架构及 Node 22.18+。Linux Web 包在 Linux 构建，不要复制 macOS 的 standalone 依赖到 Linux。也可直接用 Docker 构建 Linux Web 镜像。

源码包仅收集代码、部署脚本、文档等允许目录，排除 `.safeops`、私钥 / token / 数据库文件、`.env*`、构建产物、依赖及 Git 目录；源码符号链接会被拒绝。源代码本身仍需在发布前检查，不应将秘密硬编码到源码。

产物名称示例：

```text
dist/safe-devops-server-0.4.1-linux-amd64.tar.gz
dist/safe-devops-web-0.4.1-linux-amd64.tar.gz
dist/safe-devops-source-0.4.1.tar.gz
```

同时保留展开目录及 SHA-256 校验清单。压缩包外有 `.tar.gz.sha256`，包内有 `SHA256SUMS`。先从可信发布渠道核对摘要，再解包；摘要本身不是代码签名。已有同名产物不会覆盖，构建失败的临时目录会清理。若进程被 SIGKILL 留下 `.lock`，确认没有构建在运行后再由管理员移除该锁。

```bash
cd dist
shasum -a 256 -c safe-devops-server-0.4.1-linux-amd64.tar.gz.sha256
```

`scripts/release.sh 0.4.1` 保留为 `make release VERSION=0.4.1` 的等价入口；新版产物按组件、系统和架构拆分，不再把本机 Web 依赖放进通用 Linux 包。

GitHub 发布流程：在目标提交手动运行 `Build packages`，等待全部原生平台构建和该提交的 CI 成功，再创建指向同一提交的版本 tag 与空白 Draft Release。手动运行 `Stage release assets`，输入不带 `v` 的版本号与打包 run ID；它会核对提交 / tag / CI、校验全部 19 个包的 SHA-256、生成源码包和汇总 `SHA256SUMS`，上传后仍保持草稿，由维护者检查再发布。已有资产不会覆盖；失败的部分上传需检查清理后重新运行。

## 桌面安装包

```bash
make setup-desktop
make package-desktop VERSION=0.4.1              # 当前系统、当前架构
make package-macos VERSION=0.4.1 DESKTOP_ARCH=arm64
make package-macos VERSION=0.4.1 DESKTOP_ARCH=amd64
make package-linux VERSION=0.4.1 DESKTOP_ARCH=amd64
make package-windows VERSION=0.4.1 DESKTOP_ARCH=amd64
```

macOS 产出 `.dmg` / `.zip`，Linux 产出 `.AppImage` / `.deb`，Windows 产出 NSIS `.exe`。所有系统可指定 `amd64` / `arm64`，默认匹配构建机架构；对应系统的命令必须在该系统执行。输出到 `dist/safe-devops-desktop-版本-系统-架构/`，包含 `SHA256SUMS`。默认包未签名 / 未公证，正式对外发布在受控构建环境配置签名。

Windows PowerShell 不依赖 Make 的等价入口：

```powershell
cd desktop
npm ci
cd ..
$env:VERSION = "0.4.1"
$env:DESKTOP_ARCH = "amd64"
python scripts/package.py desktop --desktop-os windows
```

GitHub Actions 的 `Build packages` 支持手动指定版本，使用 Linux / macOS / Windows 原生 runner 生成双架构产物并上传为 workflow artifact；不自动创建 Release、上传公网或部署。Windows arm64 安装包由 Windows x64 runner 交叉打包。工作流配置存在不代表所有目标系统已完成运行验证。

## Compose 初始化与启动

先在独立审批设备生成审核私钥，将其 **Base64 公钥**保存为 `reviewer.pub`；将客户端启动器展示的 **SSH Ed25519 设备公钥**保存为 `device.pub`。管理主机只需要这两个公钥文件。

```bash
make build
make deploy-init CONFIG_DIR=.safeops/production \
  WORKSPACE=acme PROJECT=payments \
  BROKER_URL=https://broker.example.com:8443 \
  WEB_URL=https://ops.example.com \
  APPROVER_PUBLIC_KEY_FILE=/secure/reviewer.pub \
  DEVICE_PUBLIC_KEY_FILE=/secure/device.pub \
  DIAGNOSTIC_ROOT=/srv/app/logs

make deploy-up CONFIG_DIR=.safeops/production
make deploy-status
make deploy-health
make deploy-logs SERVICE=broker LOG_TAIL=100
```

`DIAGNOSTIC_ROOT` 可选。默认 SQLite；初始化时增加 `DATABASE=postgres` 选择 PostgreSQL。后续命令读取目录的 `.database`，不会根据临时 `DATABASE` 变量切换后端。已有部署不能重新初始化或直接改 `.database` 迁移。

没有 Go 工具链时，可使用已核验的本机 CLI：`SAFE_DEVOPS_BIN=/可信路径/safe-devops make deploy-init ...`。Go CLI 包不能当作 Compose 构建源码；Compose 从源码仓库或 source 包运行。`make docker-build VERSION=0.4.1` 可单独构建镜像。

启动会等待健康，超时默认 240 秒，可设 `WAIT_TIMEOUT=600`。失败会保留容器方便 `deploy-status` / `deploy-logs` 定位。`make deploy-check` 校验 Compose 与 Broker 配置，需要已构建镜像；不会打印带数据库连接密码的展开配置。

初始化默认只绑定本机地址，正式部署需配置生成目录 `.env` 中的 `SAFEOPS_BIND_ADDRESS`、DNS 和受信证书；详细步骤见 [部署指南](deployment.md)。`compose.sh` 使用实际 CONFIG_DIR 挂载配置，复制配置目录后不会继续挂载旧目录。恢复到另一套环境时还要使用新的 `COMPOSE_PROJECT_NAME` 和端口，避免与原部署共用容器或 PostgreSQL 卷。

## 日常维护、备份与升级

```bash
make deploy-restart SERVICE=web
make deploy-stop             # 保留容器和数据库
make deploy-up
make deploy-down             # 移除本部署容器、网络；保留持久卷与配置
```

不会自动执行 `down -v`、删除数据库或清理密钥。

```bash
make deploy-backup WORKSPACE=acme
make deploy-backup WORKSPACE=acme BACKUP_FILE=acme-before-upgrade.json
make deploy-upgrade VERSION=0.4.1
```

备份保存到 `CONFIG_DIR/backups/`，默认文件名含团队、UTC 时间与随机后缀，拒绝覆盖。单团队可省略 WORKSPACE；多团队须明确选择。升级自动按配置备份 **所有团队**，预构建及配置检查完成、所有备份成功后才更新容器。失败不会自动降级数据库或回滚已经执行的业务操作。明确传入 `VERSION=...` 会设置镜像标签；不传时保留 `.env` 的 `SAFEOPS_VERSION`。

恢复使用新部署、同一团队 ID 和空数据库分区。受控备份先放入新配置的 `backups/`，然后：

```bash
make deploy-stop CONFIG_DIR=.safeops/recovery
make deploy-restore CONFIG_DIR=.safeops/recovery \
  WORKSPACE=acme BACKUP_FILE=acme-before-upgrade.json
make deploy-up CONFIG_DIR=.safeops/recovery
```

恢复前检查 Broker 已停止；内置 PostgreSQL 会先单独启动并等待就绪。恢复 CLI 拒绝覆盖非空团队分区，撤销恢复的活动授权。配置、私钥和连接器防重放账本需独立受控保管；不要回退或删除连接器账本。

策略修改仍须先停止申请和连接器、等待最长授权到期、协调更新配置，再运行：

```bash
make deploy-stop
make deploy-policy-apply WORKSPACE=acme
make deploy-up
```

`deploy-upgrade` 不隐式执行策略迁移。

## Linux systemd

把匹配目标架构的 CLI 包、配置、证书安装材料带到目标 Linux 主机。先检查安装计划：

```bash
make install-check COMPONENT=connector \
  BINARY=/secure/release/bin/safe-devops \
  SHA256=从可信发布清单取得的64位摘要 \
  CONFIG=/secure/connector/connector.json
```

确认材料后以管理员权限运行相同参数的 `make install`，单独安装证书 / 私钥并运行 `check-config`。脚本不会下载软件或改防火墙、SSH key。

```bash
sudo make install COMPONENT=connector BINARY=/secure/release/bin/safe-devops \
  SHA256=可信摘要 CONFIG=/secure/connector/connector.json
sudo make service-start COMPONENT=connector
make service-status COMPONENT=connector
sudo make service-logs COMPONENT=connector
```

Broker / Reviewer 还需 `ENV_FILE=/secure/broker.env` 或 reviewer.env（SQLite Broker 可为空文件）。Web 使用 `COMPONENT=web WEB_BUNDLE=/secure/web包展开目录 ENV_FILE=/secure/web.env`。已有配置升级传 `UPGRADE=1`，旧配置保存为 `.previous.时间戳`；证书等已准备完毕时可传 `START=1` 启用并重启。普通 `install` 默认只安装，不自动启动。

无需源码的服务器可直接使用 CLI 包内 `scripts/install.sh --help`，参数与上述 Make 包装一致。systemd 服务单元负责运行前配置检查，所有目标主机仍需核验固定程序路径、文件权限和网络连通。


### 启用通用命令审核

`make shell-config` 只生成协调配置，不修改运行中的权限。必须给定现有 Broker/Connector 配置、目标 OS 用户 ID 与新的输出目录：

```bash
make shell-config BROKER_CONFIG=/secure/broker.json \
  CONNECTOR_CONFIG=/secure/connector.json SHELL_OUTPUT=/secure/shell-update \
  SHELL_DIRECTORY=/srv/app SHELL_UID=1001 SHELL_GID=1001
```

可选 `SHELL_TIMEOUT=120`、`SHELL_PROFILE=diagnostics`。root 策略需预先配置双人审批，并显式使用 `SHELL_UID=0 SHELL_GID=0 SHELL_PRIVILEGED=1`。安装执行器、策略迁移与权限边界见 [终端说明](terminal.md#在-linux-服务器启用)。
