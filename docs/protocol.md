# API 与签名协议 / Intent v4 · Lease v3

所有响应为 JSON。人员携带 `Authorization: Bearer <token>`，启用设备策略时每次请求还须携带绑定原始字节与凭证的设备签名；Connector 使用 TLS 1.3 客户端证书。Demo 才允许 Connector bearer token。AI 身份仅可访问 me、待审/巡检队列及对应决定接口，队列携带限定范围的脱敏输出。没有 Cookie、跨域授权、任意 Shell 或上传接口。

## 人员接口

- `GET /api/me`：当前身份、角色、workspace 及是否为 demo。
- `GET /api/team`：本团队不含 token/签名密钥的成员目录。
- `GET /api/projects`：当前成员可见项目，团队审计员可见本团队全部项目。
- `GET /api/catalog`：当前可见目标、任务 / 排障范围 / 变更计划与心跳。
- `POST /api/requests`：`{"target":"linux-demo","action":"system-info","reason":"INC-42 inspect kernel","ttl_seconds":300,"max_uses":1}`。
- `GET /api/requests`：可见的最近 200 条申请。
- `GET /api/requests/{id}`：读取完整不可变意图与审批状态。
- `POST /api/requests/{id}/approve`：`{"signature":"<Base64 Ed25519>"}`。
- `POST /api/requests/{id}/revoke`：撤销本人申请或本人可审批任务。
- `POST /api/requests/{id}/execute`：仅原申请人可使用 ready 授权的剩余次数，仍受期限与速率限制。
- `POST /api/requests/{id}/handoff`：申请人或独立人工审核者将 pending AI 任务转人工。
- `POST /api/requests/{id}/reject`：人工审核者提交 `{"reason":"原因"}`，终结为 rejected。
- `GET /api/reviewer/queue`：仅 AI 身份可见的最多 20 条待审请求；跳过已审核或已转人工的申请。
- `POST /api/requests/{id}/review`：AI 提交 `{"decision":"approve|escalate|reject","reason":"...","model":"...","signature":"..."}`；仅 approve 必须有有效签名。
- `GET /api/audit`：审计员读取最近 200 条事件。

未知 JSON 字段、重复文档、非 JSON Content-Type、大于 32 KiB 的请求被拒绝。申请者最多保留 20 条活动请求。内存限流按直连来源 IP 每分钟 240 次；反向代理场景会共享来源，需配合入口网关的身份/IP 限流。该限制不是分布式限流。

## Connector 接口

- `POST /api/agent/poll`：返回 `{"job":null}` 或带签名的任务；任务领取与审计先原子持久化，再返回响应。同一 Connector 最多有一条 running 任务。
- `POST /api/agent/check`：`{"request_id":"...","lease_id":"..."}` → `{"active":true}`。
- `POST /api/agent/result`：以上字段加 `output` 和 `exit_code`。最多 16 KiB 输出，一次结果写入；过期/撤销状态不会被迟到结果覆盖。

结果无法送达不触发重跑。非零退出码与取消标记失败或保留撤销/过期状态。

## 序列化与签名

服务端返回 `payload = Base64(JSON(Intent))`，JSON 字段顺序来自 Go `Intent` 结构体，无空格。签名者应校验并展示 payload 内容，不自行重排 JSON。

审批消息字节：`UTF8("safeops.approval.v1\n" + payload)`。签名为 Ed25519，Base64 标准编码。`fingerprint = hex(SHA256(JSON(Intent)))` 用于独立设备核对。`\n` 代表一个真实 LF 字节。

控制端消息字节：`UTF8("safeops.lease.v1\n" + Base64(JSON(Lease)))`。Lease 包含版本、ID、Connector ID、原始 intent payload、审批签名集合、租约截止时间和本次使用序号 use。Intent v3 还包含 workspace_id 与 project_id，二者必须匹配连接器本地范围；只有无范围的旧配置兼容 v1/v2。Intent v2 包含 max_uses，Lease v2 的 use 必须在 1..max_uses 内；v2 意图不能使用 v1 租约。签名域仍保留 v1，版本在签名内容中。

Connector 不信任控制端回传的 quorum。它使用自己的 runbook 配置计算命令摘要、匹配申请人、去除重复审批、拒绝自批、验证本地固定审批公钥并执行本地模式和门槛。AI/human 类别由本地 approver_kinds 决定，不采信 Approval.kind。auto 仅允许显式低风险本地策略，才免审核签名。

公钥为 32 字节 Ed25519 原始公钥的 Base64。CLI 私钥文件是 Go 的 64 字节 Ed25519 私钥 Base64，0600 权限。生产文件应仅留在可信签名设备，不应保存在应用仓库、CI 或开发者设备中。

## 健康与错误

`GET /healthz` 只表示进程存活；`GET /readyz` 检查各团队数据库连接。401 表示身份无效，403 表示策略或签名不符，409 表示状态冲突，429 表示限流，503 表示存储/服务故障并拒绝执行。Demo 独有 `GET /api/demo` 暴露临时演示身份与种子，且仅接受 loopback 来源；禁止把 demo 通过公开反向代理转发出去。

## 授权窗口和状态

`pending → approved → running → ready → approved` 是有限次数复用路径；用完后 completed。固定任务首次批准自动排队，ready 必须由原申请人调用 execute；排障会话批准后 ready，由原申请人提交范围内命令；并发调用只有第一次成功。failed/rejected/revoked/expired 均不可复用。每次派发消耗预算，不因命令失败或结果未知而返还。

max_uses 必须不大于管理员 policy.max_uses。高风险必须 manual 且一次；auto 只允许 low 且一次。申请 JSON 不能传 review/mode/risk 等字段覆盖策略。AI 模式支持完整 AI 或完整人工门槛；两类的部分签名不能合并。

响应包括 review、used、needs_human、assessments 和 executions。executions 保存每次使用序号、租约 ID、状态、开始/结束、退出码与输出，顶层 output/exit_code 是最近一次结果。AI reject 标记 needs_human 而非终结，人工 reject 才终结。在线服务保持不可变 intent 与 payload。

Broker 配置指纹迁移和 Connector 旧账本迁移见 [部署指南](deployment.md)。

兼容命名：内部 Go 包、JSON `agents`、`agent_id` 与 `/api/agent/*` 保留原协议名称；这些始终表示服务器连接器，不是 AI。CLI 主名称为 connector，agent 作为兼容别名。

## v0.4 终端、上下文与风控

- `POST /api/terminal/command`：`{target, profile, command, reason?, session?}`。服务端解析严格命令语法，转为已安装范围中的结构化操作；不接受客户端策略。
- `POST /api/requests` 可携带 `operation` 创建精确诊断命令，或 `steps` 创建选定顺序的变更计划。选择 diagnostics 而不传 operation 表示申请整个诊断会话。
- `POST /api/requests/{id}/command`：仅原申请人向 ready 会话提交 DiagnosticOperation；通用 execute 不能复用会话。
- Intent v4 签署 kind/scope/operation/plan。Lease v3 签署本次 operation 和 use。精确命令的 operation 必须与意图一致；会话操作必须属于已批准 scope；计划必须与 Connector 目录中的每个 argv、超时和顺序匹配。
- 请求带不可变 `risk_context`；AI 提交 `context_digest`，不匹配则拒绝决定。原有固定任务签名编码保持兼容。
- `GET /api/reviewer/risk-queue` / `POST /api/reviewer/risk-scans/{id}`：执行后风控快照及签名评估。approve 不是执行授权。
- `GET /api/risk-scans`：当前用户/可审核范围/团队审计范围内最近巡检与证据。
- `GET /api/account-locks`：可见锁定及用于独立签名的 payload/fingerprint/can_unlock。
- `POST /api/account-locks/{id}/unlock`：独立人工签名；423 表示当前账户被锁定。

lock 决定使用 `safeops.risk-decision.v1\n` + Base64(JSON({id, context_digest, decision, reason, model}))；解锁使用 `safeops.account-unlock.v1\n` + 服务端返回的锁定 payload。字段顺序由 Go 类型固定。审核 token 不能单独触发锁定或提前解锁。普通批准继续使用原 approval 签名域。

会话初次批准为 ready，不自动执行。诊断单次非零退出仍可在剩余预算内继续；计划/固定任务失败终结。锁定状态在创建/复用/会话提交、派发与活跃检查中独立检查。已发租约撤销为尽力取消，不保证回滚完成效果。

## 逐请求设备证明

启用设备策略后，所有人工 API（包括 /me 与只读查询）要求四个头：`X-Safe-Devops-Device/Time/Nonce/Signature`。签名消息为 `safeops.device-request.v1\n` 后依次换行连接 device_id、十进制 Unix 秒、16 字节小写十六进制 nonce、HTTP method、RequestURI、原始 body SHA256、Bearer token SHA256。签名为 Ed25519 raw signature 的标准 Base64，公钥以 SSH Ed25519 authorized_keys 格式登记。每次 nonce 原子持久消费，过期/未来签名拒绝。

`GET /api/device/status` 返回 required/verified；`POST /api/device/verify` 无 token，要求设备签名，仅用于凭证输入前的设备验证，不产生任何可复用登录凭据。所有后续请求重新签名并核对设备绑定的成员。API 代理保留签名头与原始 body，不允许 query string；可选 IP 检查只信任显式配置的代理链。
