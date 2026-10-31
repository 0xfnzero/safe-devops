# 防止 GitHub 失陷扩散到生产

GitHub SSH key 通常授权 Git 读写，不自动等于服务器访问权限。但同一终端常同时保存多个 SSH key、PAT、云令牌、ssh-agent 和已登录浏览器。若服务器信任其 SSH key，或 CI/CD 自动部署攻击者可修改的内容，后果就会串联。

## 本仓库提供的基线

`deploy/github-ruleset.json` 是可导入的仓库规则集参考：默认分支禁止删除/强推，要求签名、双人审批、过期审核作废、最后一次推送由其他人批准、CODEOWNERS 与 CI。需 GitHub 组织管理员导入并按实际套餐、分支和检查名称验证。并未对任何远程仓库应用设置。

`deploy/CODEOWNERS.example` 必须替换为真实的独立团队后放到 `.github/CODEOWNERS`。不存在或无权限的 owner 不会自动构成保护。组织 owner 能修改规则，必须隔离其账户与设备；空 bypass 列表不能约束平台超级管理能力。

`.github/workflows/ci.yml` 仅构建测试，默认 `contents: read`、checkout 不持久化凭据、Actions 固定 commit SHA、PR 不接触生产密钥。没有自动发布任务、生产 SSH、生产数据库连接或云部署权限。测试数据库为临时服务。服务镜像在 CI 中使用 PostgreSQL 主版本标签；严格供应链环境还需固定镜像 digest 并建立受审核的更新流程。

## 组织层配置

1. 使用组织 SSO 与抗钓鱼 MFA/passkey，单独保护组织管理员。开发日常身份不应能删除规则或更改部署环境。
2. 清理历史 PAT、deploy key、账户 SSH key 与 CI secrets。终端事件发生后必须吊销会话和令牌，不能只轮换一份 SSH key。
3. 保护 `.github/workflows`、CODEOWNERS、Dockerfile、基础镜像、依赖锁文件、构建脚本、Connector 与部署策略；工作流本身是高权限代码。
4. 禁止在生产主机部署自托管 GitHub Runner。Runner 使用临时隔离环境、无生产网络、无长效生产凭据。
5. 避免在 `pull_request_target` 运行不可信 PR 内容；不要将分支、标题、评论文本插值进 shell。CI 产物与缓存按信任域隔离。
6. 如需 OIDC 云凭据，绑定 repository ID、固定 workflow identity、ref、受保护 environment、audience；缩小 IAM 权限与生命周期。OIDC 不等于自动安全，恶意但被允许的工作流同样可换到权限。
7. Signed commit 只表示签名来源。签名密钥也在被入侵的电脑上时，不能证明代码可靠，因此仍然需要独立 review 与发布批准。

## 发布能力的后续实现约束

v0.4 **没有制品发布接口**。建议后续设计以下闭环，再接入部署：

- 从经过独立审核的不可变 commit 构建，构建器与普通 PR CI 隔离。
- 生成制品 SHA-256、来源证明、SBOM；对安全要求高的业务进行独立可重复构建或差异核验。
- 可信发布身份签名明确 digest 与目标环境，私钥不在开发 / 运维人员电脑、普通 GitHub workflow 或共享 Runner 上。
- 审批绑定同一 digest、版本、环境与变更内容；不批准可变 branch/tag/URL。
- Connector 检查发布签名、审批签名、摘要、目标及本地策略，在不可写暂存区验证并防止校验到执行之间被替换；使用固定安装器与隔离运行账户。
- 回滚使用已经验证的制品；回滚不复用过期审批。独立保护签名根、配置更新和应急权限。

只对攻击者控制的 CI 产物加签并不会证明产物安全。签名证明的是来源与完整性，审核与独立构建才用于降低恶意源码/构建步骤风险。
