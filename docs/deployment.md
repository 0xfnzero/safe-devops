# 安装与部署 v0.4

日常工作流是“选项目 → 选服务器 → 输入受控命令 / 选择任务 → 自动 / AI / 人工审核 → 执行与查看结果”。证书、成员和策略由可信管理员初始化，普通成员不需要配置 SSH，也不需要逐条审批低风险命令。

全部命令、打包平台与参数见 [Make 命令手册](commands.md)。直接执行 `make` 查看帮助；`make deploy-up` 自动选择数据库并等待健康，`make deploy-upgrade` 在更新前备份所有团队。

## 推荐布局

控制端运行 Broker + Next.js + HTTPS 网关；默认 SQLite 单机持久化。服务器运行 **Server Connector（服务器连接器）**。人工签名在独立设备，**AI Reviewer（AI 审核器）**在独立可信主机；不要把这两个信任域合并到开发 / 运维人员电脑或普通 CI Runner。

本版为自托管部署，成员凭据由管理员分发，UI 支持查看团队项目和申请审核；没有公开注册、邀请、OIDC 或在线编辑权限策略。面向公网的完整商业 SaaS 还需要身份生命周期、防滥用和独立审计等工作。

## 1. 初始化 Docker Compose

需要 Docker Engine + Compose v2、Go 1.26（或已审核的 safe-devops 二进制）。构建 Web 使用容器中的 Node.js，无需宿主 Node。所有命令从仓库根目录运行。

**在独立审批设备**生成人工签名密钥：

```bash
safe-devops keygen --out /secure/reviewer.key
```

只把输出的 Base64 审批公钥交给可信管理员；开发 / 运维人员在桌面启动器复制设备 SSH 公钥，管理员将其登记到对应成员。**在可信管理设备**执行：

```bash
make build
./bin/safe-devops init-compose --dir .safeops/production \
  --workspace acme --project payments \
  --broker-url https://broker.example.com:8443 \
  --web-url https://ops.example.com \
  --approver-public-key BASE64_PUBLIC_KEY \
  --device-public-key 'ssh-ed25519 DEVICE_PUBLIC_KEY'
```

默认 SQLite，不依赖数据库服务器或 `SAFEOPS_DATABASE_URL`。需 PostgreSQL 时，在初始化加 `--database postgres`；启动脚本自动使用 `deploy/compose.postgres.yaml`。选择已有外部 PostgreSQL 时，可使用基础 Compose 文件、编辑 Broker 的 `database` 与 `broker.env`，使用 `sslmode=verify-full` 和正确 CA，省略内置 PostgreSQL 服务。

生成目录：

```text
production/
  .env                   # Compose 路径、UID/GID、端口，无模型凭据
  .database              # sqlite 或 postgres
  broker/                # Broker JSON、签名密钥、TLS、DB 环境
  web/                   # 仅固定 Broker URL 和公共 CA
  gateway/               # Web TLS 与 Caddy 配置
  connector/             # 单台目标服务器的独立配置与 mTLS 身份
  credentials/           # developer / reviewer / auditor 的独立 token、公共 CA
  offline-ca/            # CA 私钥，不挂载到任何容器
  postgres/              # 可选 DB 密码和 TLS，DB 不发布宿主端口
  data/broker/           # SQLite 与 WAL 文件
  data/connector/        # 容器连接器防重放账本
  backups/               # 显式备份输出
```

初始化不生成或复制人工私钥，已有目录永不覆盖。密钥与文件 0600、目录 0700。Compose 非 root 服务使用生成目录所有者 UID/GID；以 root 初始化时采用 10001 并设置服务目录所有权。将配置复制到另一宿主时，应保持或重新设置 UID/GID，不能简单 `chmod 777`。

初始化生成五年 CA 和一年服务/客户端证书。`offline-ca/` 应在安全保管后移出在线主机；`credentials/` 中的 token 分发给对应成员后转移到受控秘密存储。Broker、Web 容器不挂载这两个目录。目标连接器安装完成后，其原始私钥包也应移出控制端。

## 2. 信任证书并启动

把 `credentials/ca.crt` 安装到浏览器/审核设备信任库；或使用已有企业 PKI 签发的证书替换生成证书。服务端证书 SAN 必须匹配域名，Broker 还需包含 Compose 内部 DNS `broker`，数据库需包含 `postgres`。连接器客户端证书 clientAuth 的 CN 必须精确等于配置 `id`，由专用 Connector CA 签发。

默认只发布 `127.0.0.1:443` 和 `127.0.0.1:8443`，适合本机验证。正式管理网络部署时，在生成 `.env` 增加 `SAFEOPS_BIND_ADDRESS=实际管理网IP`，设置两个域名 DNS 指向该主机，并仅允许管理网络访问。不会自动修改宿主防火墙。访问域名须与证书一致，不能跳过证书校验。

```bash
./scripts/compose.sh .safeops/production config --quiet
./scripts/compose.sh .safeops/production up -d --build
./scripts/compose.sh .safeops/production ps
./scripts/compose.sh .safeops/production logs --tail=100 broker
```

直接使用标准 Compose 等价命令：

```bash
docker compose --env-file .safeops/production/.env -f deploy/compose.yaml up -d --build
# PostgreSQL 再加：-f deploy/compose.postgres.yaml
```

所有服务健康后访问配置的 Web 域名，使用 `credentials/developer.token` 的内容登录。凭据仅在页面内存，刷新后重新登录；主题和语言偏好持久保存。生产页面不提供演示身份。Next.js 在运行时读取 `SAFEOPS_API_ORIGIN`，改地址只需重新创建 Web 容器，无需重建前端。

Broker 直接终止 mTLS；外部负载均衡需对 8443 做 L4 TLS 透传。普通 HTTP 反向代理不能传递可信客户端证书身份。Web 转发身份 token、设备签名头和原始 JSON 字节，禁止通过 Web 代理调用连接器接口。

## 3. 接入第一台 Linux 服务器

复制 `connector/` 到目标主机的受控暂存目录。选择 systemd 可管理**该宿主**允许的固定任务：

```bash
sudo ./scripts/install.sh --component connector \
  --binary ./bin/safe-devops --sha256 VERIFIED_SHA256 \
  --config ./connector/connector.json
sudo install -o safe-devops-connector -g safe-devops-connector -m 0600 \
  ./connector/connector-tls.key /etc/safe-devops/connector-tls.key
sudo install -o root -g root -m 0644 \
  ./connector/connector.crt ./connector/broker-ca.crt /etc/safe-devops/
sudo -u safe-devops-connector /usr/local/bin/safe-devops check-config \
  --role connector --config /etc/safe-devops/connector.json
sudo systemctl enable --now safe-devops-connector
```

运行前核验 `/usr/bin/uname` 等固定程序路径。连接器是无登录、无 capabilities、不能提权的账户；默认只开放一次人工批准的 uname 诊断。需要其他操作时，管理员协调安装固定任务，不开放任意 shell、sudo ALL、可写脚本或 `git pull`。

服务器只需出站访问 Broker 8443。开发者到生产 SSH / 管理端口应由安全组或防火墙阻断，历史个人 authorized_keys、云 API、VPN 与 CI 凭据也须分离。保留独立应急管理通道。safe-devops 不会替你修改这些既有访问权限。

如只需要容器内固定任务，可在目标 Docker 主机准备连接器包及可写 `data/connector` 后运行：

```bash
docker compose --env-file /secure/safe-devops/.env \
  -f deploy/compose.connector.yaml up -d --build
```

其中 `.env` 设置 `SAFEOPS_CONFIG_DIR=/secure/safe-devops`、对应 UID/GID。该容器不管理宿主机；不要添加 privileged、host PID、宿主根目录或 Docker socket 挂载。服务器连接器不需要任何入站端口。

也支持原生 Docker。先构建镜像，再使用同样配置：

```bash
docker build -f deploy/docker/server.Dockerfile -t safe-devops-server:0.4.0 .
docker run -d --name safe-devops-connector --restart unless-stopped \
  --user 10001:10001 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -v /secure/safe-devops/connector:/etc/safe-devops:ro \
  -v /secure/safe-devops/data/connector:/var/lib/safe-devops-connector \
  safe-devops-server:0.4.0 connector --config /etc/safe-devops/connector.json
```

将 UID/GID 与目录实际所有者一致。镜像构建会执行仓库代码，应从经过审核的版本构建并按组织发布策略固定镜像 digest。

## 4. 第一次申请和审批

开发或运维人员登录后选项目、服务器、任务并填写原因。默认任务需要人工审核。审核者在**独立设备**持有自己的 token 与私钥：

```bash
export SAFEOPS_TOKEN="$(cat /secure/reviewer.token)"
safe-devops approve --url https://broker.example.com:8443 --ca /secure/ca.crt \
  --request REQUEST_ID --key /secure/reviewer.key
# 核对显示的团队、项目、目标、任务摘要、原因、次数和期限后：
safe-devops approve --url https://broker.example.com:8443 --ca /secure/ca.crt \
  --request REQUEST_ID --key /secure/reviewer.key --confirm FULL_FINGERPRINT
```

第一条只预览，第二条才签名。任务执行后页面显示结果和审计。生产网页不接收人工私钥，提供独立 CLI 操作提示。

日常低风险任务可在 Broker action 与本地 Connector runbook 同时配置 `auto/low/quorum=0/max_uses=1`。AI 方式见 [审核器指南](ai-review.md)，可用独立主机的 `deploy/compose.reviewer.yaml` 或 systemd 启动；默认 Compose 不会把 AI 私钥并入控制端，也不需要模型 API key。

## 5. 多团队、多项目、多服务器

Broker 的 `workspaces[]` 每项包含独立 `id/name/private_key_file/principals/projects/agents/targets`。增加团队必须生成新的 Broker 签名根、人员 token、审批公钥和连接器证书 CN；配置校验拒绝跨团队复用 token、审批密钥或连接器身份。共享数据库按团队分区，共享进程的管理员仍是全局可信管理员。

团队内 `projects[]` 指定 `members`；`targets[].project_id` 绑定项目。一个项目可添加任意多个 target，每个 target 绑定独立连接器 ID 和证书。每台连接器固定自己的 `workspace_id/project_id/target`，不能共享原始连接器包。审核器固定 `workspace_id`，每条规则固定 `project_id`。审计员仅跨本团队项目查看，不跨团队。

参考 [broker.example.json](../deploy/broker.example.json)、[connector.example.json](../deploy/connector.example.json)。修改配置后按下面的策略更新流程生效，不通过普通网页或开发者提交的仓库内容自动同步生产策略。

## 6. 备份、迁移与恢复

在线按团队备份（输出文件必须不存在）：

```bash
./scripts/compose.sh .safeops/production exec broker safe-devops backup \
  --config /etc/safe-devops/broker.json --workspace acme --out /backups/acme-20261031.json
```

导出包含审计与操作数据，需加密并保存到独立备份系统。JSON 摘要可检测意外损坏，不抵抗拥有重算能力的攻击者；长期审计还需独立 WORM / SIEM 检查点。私钥、配置、CA 要单独受控备份。

SQLite 不可只复制运行中的 `.db` 忽略 WAL；优先使用上述事务导出。PostgreSQL 也支持平台级备份，但恢复活动授权时应走 safe-devops 的撤销逻辑。恢复目标须是**同团队 ID 的空分区**：

```bash
# 新实例已放入相同团队的配置与信任根，数据库为空，Broker 尚未启动。
./scripts/compose.sh .safeops/recovery run --rm --no-deps broker restore \
  --config /etc/safe-devops/broker.json --workspace acme --in /backups/acme-20261031.json
./scripts/compose.sh .safeops/recovery up -d
```

若目标用内置 PostgreSQL，先 `up -d postgres`，待其健康后执行 restore。restore 保留历史、撤销恢复的活动授权、清除心跳、记录恢复事件；拒绝覆盖已有分区和跨团队导入。SQLite 与 PostgreSQL 可互相导出恢复。**不能回滚或删除连接器防重放账本。**

切换数据库也需停申请、等旧授权失效、停止 Broker、备份、在新后端空分区恢复，然后启动；直接更改 `.database` 不会迁移任何数据。

## 7. 升级、策略变更和证书续期

只升级二进制、策略不变：备份 → 预构建新镜像 → `check-config` → `up -d` → 检查健康和诊断任务。不要重新运行初始化覆盖密钥。`SAFEOPS_VERSION` 可设置为组织内审定版本标签。数据库 schema 版本不匹配会拒绝启动；当前自动迁移只负责创建版本 1 表，不提供任意旧版生产数据迁移。

身份/项目/任务/签名根改变：

1. 暂停新申请并停止连接器；等待最长 15 分钟，确保已有授权过期。
2. 备份，安装协调更新后的 Broker 和每台连接器配置。
3. 停止旧 Broker；执行下列维护命令，保留已有历史。
4. 启动 Broker 和连接器，验证可见范围、签名与固定诊断。

```bash
./scripts/compose.sh .safeops/production stop broker
./scripts/compose.sh .safeops/production run --rm --no-deps broker check-config \
  --role broker --config /etc/safe-devops/broker.json
./scripts/compose.sh .safeops/production run --rm --no-deps broker policy-apply \
  --config /etc/safe-devops/broker.json --workspace acme
./scripts/compose.sh .safeops/production up -d
```

policy-apply 检查所有旧请求期限，未过期即拒绝；旧 Broker 进程也因策略指纹改变而拒绝后续操作。不能手改数据库指纹或清空审计来绕过。

证书到期前由企业 PKI 或受控离线 CA 续签，维持正确 SAN、CN 与 EKU，原子安装新证书和 0600 私钥后重启对应服务。配置文件路径不变、签名根不变时 TLS 续期不要求策略迁移。当前未实现 ACME 自动续期、CRL/OCSP 或在线密钥轮换；应设置到期告警。急需吊销失陷连接器时先通过网络隔离并停止服务，再按策略维护流程移除身份 / 更换证书 ID 与密钥。

旧 v0.1/v0.2 文件状态 / `safeops_state` 不会静默导入新分区表，应保留旧历史、等待全部授权失效、建立新范围配置；连接器保留旧账本，软件支持平面 ID 账本升级。v0.3 `workspaces[]` 使用范围签名 Intent v3，不接受无范围旧授权。

## 8. 无 Docker 安装与故障定位

已有 PKI 时使用 `safe-devops init-config`（与 init-compose 相同业务参数，不需 web-url），再安装 TLS。`make release VERSION=0.4.2` 分别生成 Linux/macOS 双架构 CLI 包、当前构建平台的 Next standalone 包和源码包；CLI/Web 包用于原生/systemd 安装，Web 必须在目标 OS/架构上构建。Docker 的 `--build` 命令从源码仓库或 source 包运行。`scripts/install.sh --help` 提供各组件安装、`--dry-run`、`--upgrade` 与 `--start`。systemd 的 Web 需要 Node 22.18+，Broker 仍可只用 SQLite。升级备份文件留在 `/etc/safe-devops/*.previous.TIMESTAMP`；脚本不改防火墙或 SSH。

- 容器权限错误：核对 `.env` UID/GID 与配置、数据目录所有者；SQLite 目录不能组/全局可写。
- Broker 不健康：检查证书 SAN、CA、DB 连接与 schema；`/healthz` 仅存活，`/readyz` 验证数据库连接，不代表所有任务可执行。
- 连接器离线：核对目标到 Broker 出站、DNS、TLS 信任、客户端证书 CN、团队与项目配置。
- 策略变更拒绝启动：执行规范的 policy-apply 流程，不删除数据绕过。
- AI 离线/不兼容：请求保持待审，转人工；先检查独立审核器日志和模型结构化输出能力。
- SQLite 应放本地可靠磁盘，不放 NFS，不通过共享 WAL 做多机 HA。PostgreSQL 虽支持事务并发，当前团队文档存储仍需控制规模与审计增长。

## v0.4 终端接入

`init-config` 和 `init-compose` 均可增加 `--diagnostic-root /srv/app/logs`，初始化即生成匹配的 Broker 与 Connector 诊断范围。目录是目标服务器上的路径；允许普通 Connector 用户读取即可，不要开放根目录、用户 HOME、密钥目录或任意挂载。初始仅有人工身份，除 ls/status 外采用人工审批；需要 AI 时为该 profile 安装独立 AI 公钥、修改双方审核策略，并配置 Reviewer rules 的一致 command_hash。

现有部署可参考 `.safeops/v4/agent.json` 中 diagnostics/plan 结构（只参考结构，不复制演示凭据），通过 `safe-devops hash-runbook --config connector.json --action diagnostics` 计算摘要，再按原 policy-apply 流程迁移。旧请求不会自动变成新范围。工作区可配置 `risk_lock_seconds`（默认 900，60..86400）。

桌面客户端只填写 Web 的 HTTPS 域名，不需要服务器密钥或连接器证书。运行和打包见 [终端文档](terminal.md)。Web 与 Broker 的 Docker/Compose 部署方式保持一致，SQLite 仍为默认。容器连接器若读取宿主日志，需要管理员明确增加单个业务日志目录的只读挂载，并匹配配置路径；默认不挂载宿主文件系统。

## 受限特权变更执行器

仅需要特权计划的 Linux 主机安装此组件。**不要**将 Connector 改成 root、赋予 sudo ALL、挂载 Docker socket 或开启 privileged 容器。

1. 创建 `/etc/safe-devops/change-runner.json`，结构为 Connector 配置，ID/workspace/project/target/Broker 公钥一致，但只保留 `plan.privileged=true` 的任务。必须是 manual/high/单次/至少双人签名；固定完整步骤目录。
2. runner 配置、TLS 私钥、公钥配置、执行二进制和依赖均由 root 管理，不可让 Connector 用户写入。配置文件 root:root 0600；使用独立 TLS 私钥/证书，客户端 CN 与该目标的连接器身份匹配，避免共享可读私钥。
3. 双方 `change_socket` 为 `/run/safe-devops-change/runner.sock`。runner 的 `ledger_file` 为 `/var/lib/safe-devops-change-runner/ledger.json`，**不能共用 Connector 账本**。runner 用自己的 mTLS 客户端验证活跃租约，独立持久消费一次使用编号。
4. 使用 `scripts/install.sh --component change-runner --binary ... --sha256 ... --config ...` 安装；模板 `safe-devops-change-runner.service` 创建 root:safe-devops-connector 的 0750 socket 目录，socket 为 0660。也可手工安装同等权限配置。先准备目标机 Connector 账户/组；预检查文件权限，测试无害计划后再开启真实变更。
5. 保持 root runner 在宿主 systemd 下运行；控制面可继续使用 Compose。runner 为执行系统变更保留必要写权限，计划的程序与软件源本身必须受信任。包管理器 hook/script 仍有 root 影响范围，不能把 argv 白名单当作沙箱。

runner 只监听 Unix socket，没有 TCP 监听或裸命令 API。配置通用命令时，它接受经过独立验证的精确 Shell 授权；非 root 使用专用 UID/GID，root 需要双人签名，详见 [通用命令部署](terminal.md#在-linux-服务器启用)。进程崩溃后若留下 socket 文件，确认旧服务确实停止后由管理员清理该 socket 再重启；不要清空持久账本。取消、断联和超时会停止后续步骤并尽力杀死正在执行的进程组；已经完成的系统修改不可自动撤销。

## 已授权机器与每次请求身份核验

新初始化命令必须增加 `--device-public-key`，从桌面启动器获取；每个成员可绑定多台已登记机器。IP 不必配置。普通浏览器无法为已启用设备策略的团队登录，CLI 需设备私钥。旧配置应显式迁移启用设备策略。配置、反向代理 IP 可信链和密钥轮换见 [设备准入文档](device-access.md)。
