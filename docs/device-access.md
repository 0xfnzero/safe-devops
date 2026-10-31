# 设备准入与逐请求签名

访问凭证只证明成员身份。启用 `device_policy.required` 后，还必须证明当前设备持有管理员登记的 SSH Ed25519 公钥对应的私钥。

**登录以及后续每一个 API 请求都验证两者。** 不是只在登录时换发一个可复制的“设备 session token”。签名绑定设备 ID、时间、一次性随机数、HTTP 方法、完整路径、原始请求体 SHA-256、身份凭证 SHA-256；请求体或凭证换一个字节都会失败。随机数在 SQLite/PostgreSQL 事务中消费，服务重启也不能重放有效窗口内的旧请求。接受最多过去 30 秒/未来 5 秒的时钟偏差。

## 首次部署

1. 在开发/运维电脑启动 Electron 客户端。启动器显示设备 ID 与 `ssh-ed25519 ...` 公钥。将**公钥**交给可信管理员；私钥不上传。
2. 管理员在初始化时传入 `--device-public-key 'ssh-ed25519 AAAA...'`。新 CLI 初始化强制要求此参数，并默认开启设备验证。也可在可信配置中添加多个已授权设备，分别绑定允许登录的成员。
3. 初始审核设备使用 `--approver-public-key` 对应的独立审核 Ed25519 密钥作为设备证明密钥，签名域与操作审批严格分离。CLI 自动使用 `approve/unlock-account --key` 提供的密钥；推荐后续给审核机器登记独立设备公钥，并设置 `SAFEOPS_DEVICE_KEY=/secure/device.key`。
4. 默认没有 IP 限制。需要时给单个设备设置 `allowed_cidrs`，例如公司出口 `203.0.113.10/32` 或 VPN `10.20.0.0/16`。设备证明仍是必需条件。

示例（真实 ID 必须由公钥推导，客户端会展示）：

```json
{
  "device_policy": {
    "required": true,
    "devices": [{
      "id": "device-公钥SHA256前16个十六进制字符",
      "name": "Operations laptop",
      "ssh_public_key": "ssh-ed25519 BASE64_PUBLIC_KEY",
      "principals": ["developer"],
      "allowed_cidrs": ["203.0.113.10/32"],
      "disabled": false
    }]
  }
}
```

放在每个 `workspaces[]` 中（旧单团队配置可放顶层）。删除 `allowed_cidrs` 或设为 `[]` 表示不限制 IP。`required:true, devices:[]` 是封闭状态，所有人工登录都被拒绝。示例中的占位公钥不能直接使用。AI 审核服务保留独立 token/签名密钥，服务器连接器使用 mTLS；二者不是人员电脑，不使用该设备准入协议。

CLI 可读取 OpenSSH Ed25519 / PKCS#8 Ed25519 私钥，或本项目 CLI 的 Ed25519 私钥格式：

```bash
safe-devops device-public-key --key /secure/device.key
export SAFEOPS_DEVICE_KEY=/secure/device.key
```

脚本集成可使用 `safe-devops device-headers --key ... --method POST --path /api/terminal/command --body-file exact-request.json`，凭证通过 `SAFEOPS_TOKEN` 环境变量提供，输出仅含设备签名头；每次调用生成新的随机数。发送的原始 body 必须逐字节相同，不能重新格式化 JSON。

## 桌面客户端中的密钥

客户端用操作系统密钥环保护加密后的设备私钥文件，签名在 Electron 主进程完成，远程页面没有 Node、preload 或导出/签名 IPC。主进程仅为已配置控制台同源 API 签名，签名原始请求字节；不读取网页传入的文件路径。没有可用系统加密保护时拒绝创建密钥，不回退明文存储。Linux 需可用的 Secret Service/KWallet，禁用 basic_text 回退。

启动器先做无凭证的设备验证，再打开凭证登录界面；后续所有请求仍重新签名。普通浏览器没有设备签名能力，受保护的控制台会提示使用已登记客户端。`--demo` 的本机演示默认不开设备准入；独立集成测试覆盖开启后的拒绝行为。

## 反向代理与可选 IP

默认只信任 TCP 连接的真实 RemoteAddr，忽略任意客户端伪造的 X-Forwarded-For。经 Web/Caddy 转发时，不能把 Web 服务的出口 IP 当作人员 IP 白名单。

若确需原始客户端 IP 限制：入口网关必须覆盖客户端自带转发头，Web 仅能由该网关访问；Web 配置 `SAFEOPS_TRUST_PROXY=true`，Broker 每个设备策略配置 `trusted_proxy_cidrs` 为实际 Web/网关的精确 IP 或受控专用子网。Broker 从右向左跳过受信代理，使用第一个不受信地址作为客户地址；受信代理缺失有效链时拒绝。不要信任 0.0.0.0/0 或整个公共网络，不要公开 Web 内部端口。默认 Compose 不发布 Web 内部端口，但需要按实际网络显式配置可信代理；未配置 IP 条件时无需这些额外设置。

## 撤销、迁移与边界

设备策略属于 Broker 策略指纹。已有部署安装策略、停用设备或更换公钥按 policy-apply 维护流程执行；旧进程检测策略不一致后拒绝继续服务。紧急情况下先使用账户风控锁定来撤销授权并停止后续操作，再完成设备/凭证轮换。旧配置缺省 device_policy 保持兼容，不会自动假装已经具备设备限制；升级时应明确启用。

这一层可以阻止**仅窃取 token、复制已签名请求、修改请求内容**后在未授权机器操作。它是密钥持有证明，**不是物理机器远程证明**：若攻击者同时取得解密后的软件设备私钥和 token，就可以冒充设备；已控制的授权机器也仍可能代签。系统密钥环保护存储，不能抵挡控制同一登录会话/内存的所有恶意程序。更高等级防克隆需要 TPM / Secure Enclave / 硬件安全密钥中的不可导出密钥，目前未实现硬件远程证明。命令范围、独立审核与账户风控仍然必须保留。
