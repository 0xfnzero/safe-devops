# 终端、会话与变更计划 / v0.4

Web 控制台和 Electron 桌面客户端共用终端。客户端只有申请身份，不建立直连 SSH，不持有服务器 SSH 私钥，也不能在本机绕过审核。服务器连接器主动出站领取签名授权。

## 直接输入命令

选择项目中的服务器与排障范围，输入：

```text
ls
ls logs:.
status
cat logs:app.log
tail -n 50 logs:app.log
grep 'ERROR' logs:app.log
run disk-usage
plan demo-change check verify
```

`logs` 是管理员命名的目录资源；也可输入位于该目录内的绝对路径。`grep` 是字面文本搜索。路径无需每个文件建模板。结果以无执行能力的纯文本显示。

主终端把命令、审核状态、输出和下一行提示符放在同一块连续区域，默认 16px 等宽字体，可通过底栏 A− / A+ 调整。服务器列表默认收起；展开后按项目选择服务器，每个打开的标签分别保留当前页面内的输入草稿。审核详情、命令参考与工单目的按需展开。提交后焦点回到提示符；向上查看旧输出时不会强制滚回底部，可点“最新输出”返回。

主终端键盘操作：

- `Enter` 提交；`Tab` 补全可用命令，多个候选可用 ↑/↓ 选择后 Tab / Enter 填入，再按 Enter 才提交。参考命令和历史命令点击后也只填入。
- ↑/↓ 浏览历史，退回最新位置恢复原草稿；`Ctrl+R` 搜索历史，Enter 填入匹配项，Esc 关闭。
- `clear` 或 `Ctrl+L` 清屏，保留审核中 / 执行中的操作，不删除历史或审计；“恢复记录”重新显示。本页刷新或重新登录也会重新读取服务器记录。
- `Ctrl+C` 在未选中文字时清除输入；输入为空时撤销当前服务器最近一条活动操作。选中会话时只作用于该会话。撤销会尽力中止执行，不能回滚已完成的效果。
- `help` 展开命令与授权范围；多行粘贴会被拒绝，不会自动执行。

草稿与历史搜索状态只在当前登录页面内保存；关闭标签、退出或刷新会清除本地草稿。详情抽屉内的简化会话输入仍保留 ↑/↓ 历史、Ctrl+L 清空输入，日常操作建议使用主终端。

服务器先识别内置诊断；无法匹配时进入**通用命令审核通道**，支持 Shell、管道、重定向、命令替换和任意已安装程序。未知命令不会直接被命令白名单拒绝，而是转人工审核。显式 `logs:path` 语法保留受限文件读取；普通路径与 `ls -la` 使用真实 Shell 语义和配置的起始目录。每条命令以 `/bin/sh -c` 独立运行，不保持上一条的 `cd` 或变量；使用 `cd /srv/app && 命令`。暂不提供 stdin 交互、vim/top 全屏 PTY 或实时输出流，输出在执行结束后返回。

`ls/status` 可由本地 `auto_tools` 明确允许后直通，其余命令单独生成不可变 Intent v4，AI/人工签署精确操作。AI 不能扩大目录、工具或特权。自动路径仍限流、签名、计数并记录输出。

## 连续排障会话

在终端点击“申请授权”，会预选当前服务器和排障范围；设置 5/10/15 分钟、最多 100 次（不超过管理员上限，初始值为 20 次或更低的策略上限），填写目的并提交。批准后进入 ready，**不会自动执行命令**。在主终端“授权方式”中显式选择已批准会话，即可在同一提示符连续输入命令；也可使用详情中的简化终端。在预先批准的整个目录/工具范围内逐次验证，不再每条审批。已选会话失效、耗尽或正在执行时停止提交。会话可用时输入通用 Shell 命令会创建新的单次审批，不消费或借用会话授权。

一次会话授权意味着被盗用的申请身份也能使用其剩余范围、期限与次数；需要每条独立 AI 评估时使用默认逐命令终端。会话命令仍进入行为巡检，可触发账户锁定。读取不存在的日志不会关闭会话，但仍消耗一次预算；撤销、过期或次数用尽不可复用。

Go `os.Root` 限制目录穿越和符号链接越界；只读取普通文件，FIFO、设备被拒绝。每次扫描最多 2 MiB、输出最多 16 KiB、目录最多 1000 项、tail 最多 200 行。范围应是本地、管理员控制的业务日志目录，不能包含凭据、私钥、硬链接到敏感文件或不可信挂载；文件系统读取的内核阻塞不是可强制撤销的任意远程文件系统沙箱。

## 安装与服务变更

管理员在 Connector 安装 `plan.steps`，每步包含 ID、说明、绝对 argv 和超时，Broker 只暴露同一目录。用户选择最多 8 步并调整顺序，一次审批绑定完整 argv、顺序与总超时。失败立即停止，不自动重试，不声称可以回滚已完成的效果。

特权计划要求 `manual/high/max_uses=1/quorum>=2/privileged=true`。Connector 保持普通用户，通过 Unix socket 将**原始签名信封**交给独立 root 变更执行器；执行器使用自身配置、审核公钥、防重放账本和 mTLS 租约检查重新验证。它不接受裸 argv。见 [部署指南](deployment.md#受限特权变更执行器)。

需要安装包时，管理员定义明确软件包/版本和受控软件源，不开放任意包名或下载 URL。软件包维护脚本具有相应特权，精确 argv 不能保证包内容安全。演示计划只执行 uname/uptime，不安装软件。

## Electron 客户端

仓库根目录可用 `make setup-desktop`、`make desktop`、`make dev-desktop`，以及 `make package-macos` / `package-windows` / `package-linux`。各平台要求和产物路径见 [命令手册](commands.md#桌面安装包)。

```bash
cd desktop
npm ci
npm run dev       # 开发模式允许显式 loopback HTTP，连接本机 make demo
npm test
npm run pack      # 本平台可运行目录
npm run dist      # 本平台分发包；macOS dmg/zip、Windows NSIS、Linux AppImage/deb
```

生产客户端输入团队 **Web 控制台 HTTPS 地址**，不是 Broker 地址。记住控制台域名；设备私钥通过操作系统密钥环加密保存，不持久化登录 token 或人工审核私钥。生产要求先登记设备公钥，每次请求独立签名，见 [设备准入](device-access.md)。需在操作系统信任公司的 TLS CA，不允许跳过证书校验。远程窗口使用 sandbox/contextIsolation，禁止 Node、preload、webview、权限请求、下载、新窗口和跨域导航/网络请求；启动器只有固定的 settings/connect IPC。登录仍在 Web 页面内存中。

当前打包脚本默认不自动调用开发机的签名身份。对外发行前须在受控发布环境配置代码签名与 macOS notarization，并验证各目标操作系统。已提供跨平台打包配置，不代表 Windows/Linux 安装包已经在本机实际测试。没有自动更新通道。


## 通用命令：规则、AI 与人工的边界

- 已配置目录内的内置 `ls/status` 保留原规则。通用通道的 `ls`（无路径，仅支持 `-l/-a/-la/-al` 精确形式）、`pwd`、`whoami`、`uname`、`uname -a/-s/-srm` 仅在完整文本精确匹配且非 root 时可快速放行。
- `id`、`uptime`、`df -h`、`free -h`、`ps aux`、`ps -ef` 只有管理员启用 AI 审核时才能由 AI 批准；人工策略不会被自动改成 AI。
- 其他文本全部可以提交，包括 `apt-get install nginx`、`curl ... | sh`、文件写入和自定义脚本，但默认标为高风险并转独立人工。这里限制的是**自动批准权限**，不是可提交命令集合。
- root 策略下任何通用命令都要两名独立人工签名，不能让 AI 代签。普通账户不能用 `sudo` 或 setuid 文件提升权限；需要系统级安装时选择管理员配置的 root 策略。
- 请求签名覆盖原始命令、起始目录、UID/GID、超时、租户、项目、目标和有效期。每次只执行一次；更改文本、目录、账户或者重复使用都要重新申请。
- 审核 UI 显示“已转人工”、原始命令及执行账户。执行结果进入原来的审计和 AI 行为巡检，异常行为仍可锁定账户。

### 在 Linux 服务器启用

控制面仍可用 Docker Compose + SQLite/PostgreSQL。通用命令执行器安装在目标 Linux 主机上；控制面容器不挂载 Docker socket、不获得目标主机 root。目标需 `/bin/sh` 和 util-linux 的 `/usr/bin/setpriv`。

1. 在目标机创建专用执行用户（例如 `safe-devops-exec`，禁止交互登录），只授予需要管理的业务文件权限。不要复用 Connector/Reviewer 用户；不要加入 Docker、sudo 或其他管理组。准备初始目录，例如 `/srv/app`。
2. 以现有已安装的 `diagnostics` 配置生成协调更新。初次初始化应传入 `--diagnostic-root /srv/app/logs`。UID/GID 使用目标机 `id -u safe-devops-exec` / `id -g safe-devops-exec` 的实际值：

   ```bash
   ./bin/safe-devops configure-shell \
     --broker-config /secure/current/broker.json \
     --connector-config /secure/current/connector.json \
     --out /secure/reviewed-shell \
     --directory /srv/app --uid 1001 --gid 1001 --timeout 120
   ```

   只生成新目录下 `broker.json`、`connector.json`、`change-runner.json`，不修改运行配置或密钥。若要 root 通道，先为这个 profile 配好至少两名独立人工及 `quorum >= 2`，再使用 `--uid 0 --gid 0 --privileged`。
3. 将文件分发至各自机器。执行器复用该目标的 mTLS 身份与公钥，但配置/账本由 root 保管。通过 `scripts/install.sh --component change-runner`（或 `make install COMPONENT=change-runner ...`）安装已有 systemd 服务。Connector 仍以低权限账户运行；Unix socket 为 root 所有、Connector 组可访问。
4. 新策略摘要会变化：先停 Broker 并备份，再按部署指南运行 `policy-apply`；它撤销旧活动授权。将同一 profile 摘要更新到独立 AI Reviewer 的规则后重新启动各服务。未更新的 AI 规则只能转人工，不能放宽权限。现有部署不会被自动启用通用命令。

独立执行器再次验证原始授权信封、本地配置、人工/AI 签名、租约、账号状态和防重放账本。非 root 命令丢弃附加组和所有 Linux capabilities，并开启 `no_new_privs`；凭据不经环境传入。**起始目录不是隔离目录**，实际文件权限由该 UID 控制；执行用户不能拥有 Connector 的凭据文件。命令有超时、进程组取消和输出上限，已发生的写入无法回滚，主动脱离进程组的子进程也不保证能被取消。需要强资源/网络隔离时，执行目标应置于专用 VM/container 中。

批准 root 命令或下载的程序，意味着信任其完整行为：它可能修改系统、持久化或读取机密。精确签名保证命令未被替换，不保证脚本、包仓库、符号链接和依赖在批准后未变；敏感变更应使用固定摘要的工件与双人复核。

新建 `init-demo` 环境支持同样的路由，但通用命令会以启动者的本机非 root 账户执行，**不提供生产执行账户隔离**，演示身份与审核密钥也不是安全边界。生产配置拒绝 `shell.demo=true`。旧 `.safeops/v4` 不会被覆盖；使用新的 `DEV_DIR` 启动新版演示。
