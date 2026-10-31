# v0.4.1 开发与发布验证记录

日期：2026-10-31（Asia/Shanghai），macOS arm64。本记录区分实际完成的本地 / GitHub Actions 检查与尚需目标环境验证的项目，不代表独立安全审计。

## 已完成

- 通用命令：配置 Shell 执行策略后支持提交管道、重定向、脚本和安装命令；保留原始命令与原生 `/bin/sh -c` 语义，使用精确单次签名。真实演示验证 `ls -la` 规则直通、`df -h` 模拟 AI、管道命令独立人工批准后执行。回归测试覆盖审批后篡改命令 / 目录 / UID、伪造 AI 签名、借用诊断会话授权、继承敏感环境及生产误用 demo 模式。Linux CI 已以真实不同 UID 执行，确认组 / capabilities 清空、no_new_privs 生效且无法读取执行器私有凭据。
- 下拉与输入：统一可访问的 Radix Select，验证键盘选择、Esc 返回原表单、深浅色菜单、输入框单层聚焦边框；TypeScript、ESLint 与生产构建通过。
- Docker Compose：GitHub Linux runner 已实际构建并运行 SQLite / PostgreSQL 两套栈，完成 HTTPS Web 代理、mTLS、逐请求设备签名、独立审批、命令执行、备份与重启（[运行记录](https://github.com/0xfnzero/safe-devops/actions/runs/41151101435)，两项 containers 作业通过）。网关保留 `cap_drop: ALL` 和 `no-new-privileges`；镜像移除不需要的低端口文件 capability。

- Make / 发布工具：默认 `make` 与 `make help` 输出一致，所有公开命令有说明；17 项脚本测试覆盖打包排除秘密文件、失败清理、禁止覆盖、部署目录空格、版本固定、备份失败阻止升级、多团队全部备份、恢复拒绝运行中的 Broker、SQLite 不启动 PostgreSQL、保留持久卷。实际完成 Linux/macOS amd64/arm64 四种 Go CLI 打包、本机 macOS arm64 Web standalone 与未签名 Electron DMG/ZIP 构建；版本标签为 `0.4.1-devtools`，不代表正式发布。SQLite / PostgreSQL 两种 `make deploy-init` 和 Linux Connector 的 `make install-check` 通过；隔离端口上实际启动 Broker、两个 Connector 和模拟 Reviewer，Ctrl+C 后子进程退出，端口可重新使用。GitHub 原生平台打包矩阵已实际完成 Linux/macOS amd64/arm64 的 CLI/Web 包及 Linux/macOS/Windows amd64/arm64 桌面安装包构建（[运行记录](https://github.com/0xfnzero/safe-devops/actions/runs/41150663329)）。安装包构建成功不代表每个平台已完成安装与密钥环运行验证。
- 终端交互改版：主界面使用连续命令 / 输出 / 提示符区域、16px 默认等宽字体和可收起服务器列表。浏览器实际完成规则直通 `ls`、模拟 AI 审核 `grep`、Tab 补全、↑/↓ 历史与草稿恢复、Ctrl+R 搜索、help 参考填入、Ctrl+L 清屏；提交后焦点回到输入行。新会话入口预选当前服务器 / 排障范围，申请两次额度后从主终端执行 `ls` / `status`，额度用尽时禁用提交且不自动切换授权方式。中英文 / 深浅色、390px 和 1280px 宽度下均无页面横向溢出；TypeScript、ESLint、生产构建通过。截图：`docs/screenshots/terminal-continuous-dark.png`、`docs/screenshots/terminal-continuous-light-en.png`。本轮仅更新 Web 界面和文档，现有 v0.4.0 归档未重打包。
- 本轮新增设备验证：每次人工 API 请求要求凭证绑定的 Ed25519 签名；无设备证明、身份 / 路径 / 方法 / 请求体被修改、错误机器、过期 / 超前时钟、重放均拒绝。设备预验证不授权后续请求，也不授予其他成员 / 团队权限。
- SQLite 和临时 PostgreSQL：两个 Broker / 数据库连接并发提交同一签名，仅一次成功；关闭连接、重建 Broker 后继续拒绝重放，新的请求正常通过。可选 CIDR、伪造 XFF、受信代理链和变更源端口绕过限流有回归用例。
- `scripts/test-local-tls.py` 已用开启设备策略的配置重跑：token-only 拒绝、逐请求签名经生产 Next.js 代理、独立人工审批、mTLS 连接器执行均通过。
- Electron 原生客户端：在隔离的强制设备验证环境完成设备预验证、凭证登录、POST `ls` 与 GET 输出；退出 / 重启客户端后设备公钥保持一致，重新登录可读结果。普通浏览器显示设备登记提示并禁用凭证登录。仅客户端入口使用开发模式 loopback HTTP，Next → Broker 与 Connector 使用真实 TLS/mTLS。
- AI 护栏：模拟恶意模型始终 approve，越界工具 / 路径 / 目录、重复命令预算和变更计划仍在模型调用前拒绝。模型工具调用（含 function_call）、未知 / 重复 / 大小写变体字段、指定其他锁定用户、改锁定时长或要求解锁均转人工；AI 服务身份不能创建命令、执行、改成员或解锁。
- `scripts/smoke-terminal.py`：受控命令、AI 近期输出上下文、路径穿越拒绝、排障会话、行为巡检锁定、SQLite 重启保持锁定、独立人工解锁、双人人工顺序变更计划完整闭环通过。
- 诊断文件访问：符号链接越界、FIFO / 非普通文件、目录穿越、扫描与输出上限；Go 属性测试覆盖合法路径和穿越组合。独立特权执行器校验本地计划、双人签名及自身重放账本。
- Electron 4 项 Node 测试通过；macOS arm64 `.app` 本地打包成功，包含逐请求签名实现，未签名 / 未公证。截图：`docs/screenshots/v4-device-terminal-dark.png` 与 `v4-device-terminal-light-en.png`。

- Go 1.26：所有包 `go test -race ./...` 与 `go vet ./...` 通过；Linux amd64 / arm64、CGO_ENABLED=0 静态二进制打包成功。
- SQLite：WAL/FULL、并发事务无丢失、持久重启、回滚、团队分区、未知 schema 拒绝、备份恢复撤销活动授权。
- 临时 PostgreSQL：旧存储与新团队分区后端均运行集成测试；两个实例并发 20 次事务无丢失，跨团队隔离，PostgreSQL 备份恢复到 SQLite 并保留审计。
- 初始化：SQLite/PostgreSQL 配置生成、证书 SAN/EKU/CN 验证、0600/0700 权限、不覆盖目录、不生成人工私钥；生产 SQLite 不需要 PG 环境变量；不安全 PG TLS 配置被拒绝。
- 策略变更：未过期授权拒绝迁移；过期后保留历史并记录新指纹；旧 Broker 实例拒绝继续操作。损坏与跨团队备份拒绝恢复。
- 租户与项目：相同成员/目标 ID 在不同团队隔离；跨项目成员不可操作；客户端 tenant header/body 不能改变租户；连接器拒绝错误团队或项目签名。
- 授权：自批、重复公钥凑数、跨请求签名、错误身份、过期、撤销、并发领取、次数篡改、AI 冒充人工、混合凑门槛均有负向用例。
- 连接器：持久账本拒绝重启重放，保留旧账本记录，执行次数与速率独立限制，超时中止，不继承父进程敏感环境变量。
- AI：本地 HTTP 模型模拟服务经过真实 Reviewer 签名、Broker 和连接器执行；拒绝、截断、工具调用、格式异常、低置信度、HTTP 错误均不签名。
- 实际 CLI 演示：预览不签名、错误指纹拒绝、人工签名执行；固定 ls 自动放行；模拟 AI 批准、两次复用、人工接管闭环。
- `scripts/test-local-tls.py`：使用新生成的私有测试 PKI，**不加 demo 标志**运行真实 Broker 和连接器；生产 Next.js 使用运行时 HTTPS 上游，完成独立签名任务。无客户端证书不能轮询；Web 不代理连接器接口；生产不暴露 demo 数据。所有测试进程退出后清理，私有测试材料留在忽略目录。
- Docker Compose **v2.39.4 官方解析器**：SQLite 与 PostgreSQL 两套生成配置（包含容器连接器叠加）通过 `config --quiet`。安装/启动脚本通过 shell 语法检查。
- Web：TypeScript、ESLint、Next.js 生产构建通过。页面统一名称 safe-devops，成员称为开发 / 运维成员。
- 浏览器：中文/英文、浅色/深色、刷新保留偏好但不保留身份 token；Alice 只看支付项目、Dave 只看沙箱项目。固定目录任务完成，移动端 AI 模拟审批后 1/2 → 手动复用 → 2/2 完成，复用入口消失。
- 响应式：390px 下页面 scrollWidth/clientWidth 均为 390，申请表单可用；桌面与移动截图位于 `docs/screenshots/`。

## 未在当前环境完成

- 本机没有 Docker Engine；容器构建和运行验证由上述 GitHub Linux runner 完成，仍需在实际部署主机验证其网络、磁盘权限和备份策略。
- systemd 安装只检查语法与 dry-run，没有在真实 Linux 主机安装服务。
- Windows / Linux 桌面包已构建，实际安装 / 操作系统密钥环兼容矩阵、macOS 签名 / 公证未验证；没有 TPM / Secure Enclave 不可导出设备密钥或硬件远程证明。
- 未调用真实模型供应商，也未评估模型判断准确性。演示明确标记本地模拟器。
- 已建立公开 GitHub 项目和发布流水线；未修改生产服务器或 GitHub 组织策略。没有独立安全审计、压力测试、灾备演练、HA 或公网 SaaS 上线认证。

当前按团队保存完整状态文档，审计会增长；部署需控制规模并验证容量。身份配置仍由管理员维护，没有成员邀请 / OIDC / 在线权限编辑。详细限制见 [架构](architecture.md) 和 [部署](deployment.md)。
