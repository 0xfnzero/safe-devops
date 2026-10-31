"use client";

import { useEffect, useSyncExternalStore } from "react";
import { Languages, Moon, Sun } from "lucide-react";

const en = {
  deviceRequired:
    "An enrolled device is required. Open the desktop client and ask your administrator to enroll its SSH public key before signing in.",
  deviceChecking: "Checking device access…",
  risk: "Account risk",
  riskSub: "Behavior assessments, account locks and independent recovery.",
  lockDecision: "Account locked",
  terminal: "Terminal",
  terminalSub: "Command input, layered review and traceable output.",
  handoffNotice: "Your request has been handed to human reviewers.",
  overview: "Overview",
  requests: "Access requests",
  assets: "Servers",
  team: "Team & projects",
  policy: "Access policies",
  audit: "Audit log",
  workspace: "Workspace",
  operations: "OPERATIONS",
  security: "SECURITY",
  signOut: "Sign out",
  refresh: "Refresh",
  create: "Request access",
  close: "Close",
  cancel: "Cancel",
  submit: "Submit request",
  copy: "Copy",
  copied: "Copied",
  all: "All requests",
  online: "Online",
  offline: "Offline",
  connected: "Connected",
  disconnected: "Connection interrupted",
  pending: "Awaiting review",
  approved: "Queued",
  running: "Running",
  ready: "Ready to reuse",
  completed: "Completed",
  failed: "Failed",
  revoked: "Revoked",
  expired: "Expired",
  rejected: "Rejected",
  auto: "Preauthorized",
  ai: "AI review",
  manual: "Human review",
  low: "Low risk",
  medium: "Medium risk",
  high: "High risk",
  welcome: "Your infrastructure. Clear boundaries.",
  welcomeSub:
    "Keep production access bounded when a developer or operator’s workstation is compromised.",
  loginTitle: "Sign in to safe-devops",
  loginSub: "Use the identity credential provided by your team administrator.",
  credential: "Access credential",
  signIn: "Sign in",
  sessionOnly:
    "Credentials stay in this page’s memory. Signing keys belong on independent trusted devices.",
  demoIdentities: "Explore with a demo identity",
  demo: "LOCAL DEMO",
  demoNote:
    "Disposable local identities. AI review is simulated; no model service is called.",
  switchIdentity: "Demo identity",
  overviewTitle: "Operations overview",
  overviewSub:
    "Keep routine work moving. Keep production access under control.",
  requestSub: "Every grant has a fixed scope, an expiration and a use budget.",
  assetsSub:
    "Servers connect outbound and enforce their own authorization policy.",
  teamSub:
    "One workspace. Multiple projects. Explicit membership and server access.",
  policySub: "Administrator policies decide how an operation is reviewed.",
  auditSub: "Trace authorization and execution within this workspace.",
  managed: "Managed servers",
  pendingMetric: "Waiting for review",
  activeMetric: "Active grants",
  historyMetric: "Visible requests",
  connectorOnline: "connectors online",
  noExecution: "No execution permission yet",
  boundedWindow: "Bounded by time and use count",
  recent200: "Latest 200 · current identity scope",
  recent: "Recent requests",
  requestTableSub:
    "Commands, sessions and change plans. Traceable decisions, bounded execution.",
  taskTarget: "Task / server",
  requester: "Requester",
  review: "Review",
  status: "Status",
  created: "Created",
  updated: "Auto-refresh",
  noRequests: "No requests here yet",
  noRequestsSub: "Choose a server and an installed task to get started.",
  preauthorized: "No per-run review",
  humanNeeded: "Needs human review",
  uses: "uses",
  servers: "servers",
  members: "members",
  projects: "projects",
  project: "Project",
  allProjects: "All visible projects",
  defaultProject: "Default project",
  fixedTasks: "Installed tasks",
  connector: "Server Connector",
  connectorState: "Connector status",
  lastSeen: "Last heartbeat",
  never: "Not connected yet",
  environment: "Environment",
  teamTitle: "Team members",
  member: "Member",
  identityType: "Identity type",
  role: "Role",
  projectScope: "Visible project scope",
  humanIdentity: "Human",
  aiIdentity: "AI Reviewer",
  requesterRole: "Developer / Operator",
  approverRole: "Reviewer",
  auditorRole: "Workspace auditor",
  allTeamProjects: "All workspace projects",
  managedConfig:
    "Membership and policies are managed by your trusted administrator.",
  newOperation: "NEW REQUEST",
  createTitle: "Request scoped access",
  immutable: "Scope is locked after submission. Changes require a new request.",
  target: "Target server",
  task: "Installed task",
  reason: "Reason / ticket reference",
  reasonHint:
    "Describe the diagnostic or reference the maintenance ticket. Optional for preauthorized tasks.",
  maximumUses: "Maximum executions",
  expiry: "Grant duration",
  minutes: "minutes",
  once: "1 execution",
  seconds: "seconds per run",
  max: "up to",
  grantSubmitted: "Request submitted under the configured review policy.",
  detail: "Request details",
  scope: "Workspace / project",
  expiresAt: "Expires at",
  executionCount: "Execution budget",
  fingerprint: "Intent fingerprint",
  signatureScope:
    "The signature binds workspace, project, target, task, requester, reason, expiry and maximum executions.",
  approvals: "Approval signatures",
  noSignatures: "No independent signature yet.",
  autoSignatureNote:
    "This fixed low-risk task was preauthorized by an administrator.",
  assessments: "Review decisions",
  approveDecision: "Approved",
  escalateDecision: "Escalated",
  rejectDecision: "Declined",
  manualHandoff: "Request human review",
  handedOff: "Handed off to a human reviewer.",
  humanWarning:
    "No execution permission has been granted. A human must review the complete scope and sign.",
  grantReady: "This grant can be reused",
  sameScope:
    "Only this server and fixed task. Each additional run must be triggered explicitly.",
  executeAgain: "Run again · no new review",
  queuedAgain: "Another execution was queued and will consume one use.",
  demoSign: "Sign as the current demo reviewer",
  independentSign: "Independent device approval",
  signNote:
    "Verify the complete scope and fingerprint on a trusted device before signing.",
  offlineSignature: "Offline signature",
  signaturePlaceholder: "Base64 Ed25519 signature",
  approve: "Approve and sign",
  submitSignature: "Submit signature",
  signed: "Independent signature submitted.",
  rejectReason: "Reason for rejection",
  rejectPlaceholder: "Explain why this operation should not proceed",
  reject: "Reject request",
  rejectedNotice: "Request rejected.",
  output: "Latest execution output",
  noOutput: "No output",
  executionHistory: "Execution history",
  run: "Run",
  waitingResult: "Waiting for a result",
  revoke: "Revoke grant",
  revokeNote:
    "Cancellation is best effort. Completed effects cannot be rolled back.",
  revokedNotice: "Grant revoked. Completed effects cannot be rolled back.",
  policyAutoTitle: "Routine work, preauthorized",
  policyAuto:
    "Low-risk fixed diagnostics need no per-run review. The connector enforces the allowlist, requester scope and local rate limit.",
  policyAITitle: "AI first, human fallback",
  policyAI:
    "An independent reviewer may sign allowlisted tasks. Model errors, uncertainty or refusal escalate to humans. AI and human votes cannot be combined.",
  policyHumanTitle: "Sensitive work, human review",
  policyHuman:
    "High-risk tasks require human review for every execution. Other tasks may use a bounded authorization window.",
  boundaryTitle: "Separate the trust boundaries",
  developerBoundary: "Developer device",
  developerBoundarySub:
    "Requests a fixed operation. Holds no production SSH key.",
  reviewerBoundary: "Review environment",
  reviewerBoundarySub:
    "Human and AI reviewers use independent identities and signing keys.",
  connectorBoundary: "Server Connector",
  connectorBoundarySub:
    "Validates local policy and signed scope before executing a fixed task.",
  policyCaution:
    "A compromised developer can still use preauthorized tasks and remaining uses of a valid grant. AI decisions are not proof that an operation is safe.",
  githubTitle: "Keep repository access separate from production access",
  githubNote:
    "Remove direct SSH and CI production credentials. Repository scripts must not become server commands automatically.",
  event: "Event",
  actor: "Actor",
  time: "Time",
  chain: "Hash chain",
  auditRestricted: "Workspace auditor access required",
  auditRestrictedSub:
    "An auditor can inspect this workspace’s requests and event history.",
  noEvents: "No audit events yet",
  auditCaution:
    "A hash chain detects local changes; use independent immutable storage to protect against complete history rewrites.",
  lightMode: "Switch to light theme",
  darkMode: "Switch to dark theme",
  language: "Switch language",
  reviewProgress: "Review progress",
  rootBoundary: "Fixed tasks. Independent trust.",
  footer: "Self-hosted access control",
  cliRequired:
    "Use the CLI on an independent trusted device, or paste its signature.",
  clipboardError: "Clipboard unavailable. Select and copy the value manually.",
} as const;
type Key = keyof typeof en;
const zh: Record<Key, string> = {
  deviceRequired:
    "需要已授权的设备。请打开桌面客户端，将设备 SSH 公钥交给管理员登记后，再使用访问凭证登录。",
  deviceChecking: "正在验证设备访问条件…",
  risk: "账户风控",
  riskSub: "行为评估、风险账户锁定与独立人工恢复。",
  lockDecision: "锁定账户",
  terminal: "终端",
  terminalSub: "直接输入命令，分层审核，追溯输出。",
  handoffNotice: "你的操作已转交人工审核，等待独立审核者处理。",
  overview: "工作台",
  requests: "访问申请",
  assets: "服务器",
  team: "团队与项目",
  policy: "访问策略",
  audit: "审计日志",
  workspace: "工作空间",
  operations: "运维管理",
  security: "安全管理",
  signOut: "退出登录",
  refresh: "刷新",
  create: "发起操作申请",
  close: "关闭",
  cancel: "取消",
  submit: "提交申请",
  copy: "复制",
  copied: "已复制",
  all: "全部申请",
  online: "在线",
  offline: "离线",
  connected: "已连接",
  disconnected: "连接中断",
  pending: "等待审核",
  approved: "等待执行",
  running: "执行中",
  ready: "可再次执行",
  completed: "已完成",
  failed: "失败",
  revoked: "已撤销",
  expired: "已过期",
  rejected: "已拒绝",
  auto: "自动放行",
  ai: "AI 审核",
  manual: "人工审核",
  low: "低风险",
  medium: "中风险",
  high: "高风险",
  welcome: "生产访问，有明确边界。",
  welcomeSub:
    "即使开发或运维人员的日常电脑失陷，生产操作仍受预设范围与独立校验约束。",
  loginTitle: "登录 safe-devops",
  loginSub: "使用团队管理员分配的身份凭据。",
  credential: "访问凭据",
  signIn: "登录工作台",
  sessionOnly: "凭据仅保存在当前页面内存中。签名密钥应保管在独立可信设备上。",
  demoIdentities: "使用演示身份体验",
  demo: "本机演示",
  demoNote: "临时本地身份。AI 审核为模拟流程，未调用模型服务。",
  switchIdentity: "演示身份",
  overviewTitle: "运维工作台",
  overviewSub: "让开发与运维协作更顺畅，让生产权限始终受控。",
  requestSub: "每份授权都有固定范围、截止时间和执行次数。",
  assetsSub: "服务器主动出站连接，并独立执行本地授权策略。",
  teamSub: "一个团队，多个项目。成员与服务器访问范围清晰分配。",
  policySub: "由管理员预设策略决定每项操作的审核方式。",
  auditSub: "追溯当前团队的授权与执行过程。",
  managed: "受管服务器",
  pendingMetric: "等待审核",
  activeMetric: "活动授权",
  historyMetric: "可见申请",
  connectorOnline: "个连接器在线",
  noExecution: "尚未授予执行权限",
  boundedWindow: "受有效期与次数限制",
  recent200: "最近 200 条 · 当前身份范围",
  recent: "最近的操作申请",
  requestTableSub: "固定任务、可追溯决策、有次数上限的执行。",
  taskTarget: "任务 / 服务器",
  requester: "申请人",
  review: "审核方式",
  status: "状态",
  created: "申请时间",
  updated: "自动同步",
  noRequests: "暂无申请",
  noRequestsSub: "选择服务器和已安装的固定任务，即可开始。",
  preauthorized: "无需逐次审核",
  humanNeeded: "等待人工审核",
  uses: "次",
  servers: "台服务器",
  members: "位成员",
  projects: "个项目",
  project: "项目",
  allProjects: "所有可见项目",
  defaultProject: "默认项目",
  fixedTasks: "已安装任务",
  connector: "服务器连接器",
  connectorState: "连接器状态",
  lastSeen: "最近心跳",
  never: "尚未连接",
  environment: "环境",
  teamTitle: "团队成员",
  member: "成员",
  identityType: "身份类型",
  role: "角色",
  projectScope: "可见项目范围",
  humanIdentity: "人工成员",
  aiIdentity: "AI 审核器",
  requesterRole: "开发 / 运维成员",
  approverRole: "审核成员",
  auditorRole: "团队审计员",
  allTeamProjects: "团队全部项目",
  managedConfig: "成员和策略由可信管理员管理。",
  newOperation: "新建申请",
  createTitle: "申请有范围的操作授权",
  immutable: "提交后范围将被锁定，修改需要重新申请。",
  target: "目标服务器",
  task: "已安装任务",
  reason: "操作原因 / 工单编号",
  reasonHint: "说明诊断目的或填写工单编号。自动放行任务可留空。",
  maximumUses: "最多执行次数",
  expiry: "授权有效期",
  minutes: "分钟",
  once: "1 次",
  seconds: "秒 / 次",
  max: "最多",
  grantSubmitted: "申请已提交，将按预设审核策略处理。",
  detail: "申请详情",
  scope: "团队 / 项目",
  expiresAt: "授权截止时间",
  executionCount: "执行次数",
  fingerprint: "操作指纹",
  signatureScope:
    "签名绑定团队、项目、服务器、任务、申请人、原因、有效期和执行次数。",
  approvals: "批准签名",
  noSignatures: "尚未收到独立签名。",
  autoSignatureNote: "管理员已预先允许此固定低风险任务。",
  assessments: "审核结论",
  approveDecision: "批准",
  escalateDecision: "转人工",
  rejectDecision: "拒绝",
  manualHandoff: "转交人工审核",
  handedOff: "已转交人工审核。",
  humanWarning: "尚未授予执行权限，需要人工核对完整范围后签名。",
  grantReady: "授权窗口仍有效",
  sameScope: "仅限当前服务器和固定任务，每次复用都需要主动触发。",
  executeAgain: "再次执行 · 无需重新审核",
  queuedAgain: "已排队执行，将消耗一次授权。",
  demoSign: "使用当前演示审核身份签名",
  independentSign: "独立设备审批",
  signNote: "请在可信设备上核对完整范围与指纹，再签名批准。",
  offlineSignature: "离线签名",
  signaturePlaceholder: "Base64 Ed25519 签名",
  approve: "确认并签名批准",
  submitSignature: "提交签名",
  signed: "独立签名已提交。",
  rejectReason: "拒绝原因",
  rejectPlaceholder: "说明为什么不应执行此操作",
  reject: "拒绝申请",
  rejectedNotice: "申请已拒绝。",
  output: "最近一次执行输出",
  noOutput: "无输出",
  executionHistory: "执行记录",
  run: "执行",
  waitingResult: "等待结果",
  revoke: "撤销授权",
  revokeNote: "撤销将尽力中止执行，无法回滚已经产生的效果。",
  revokedNotice: "授权已撤销，已经产生的效果无法回滚。",
  policyAutoTitle: "常规任务，自动放行",
  policyAuto:
    "低风险固定诊断无需逐次审核。连接器仍校验白名单、申请人权限，并在本地限流。",
  policyAITitle: "AI 优先，人工接管",
  policyAI:
    "独立审核器只能签署白名单内的任务。模型异常、不确定或拒绝时转人工。AI 与人工票数不能混合。",
  policyHumanTitle: "敏感操作，人工审核",
  policyHuman:
    "高风险任务每次重新审核。普通任务可在指定时间和次数内复用已批准的授权。",
  boundaryTitle: "独立的信任边界",
  developerBoundary: "开发者设备",
  developerBoundarySub: "提交命令与变更申请，不持有生产 SSH 私钥。",
  reviewerBoundary: "独立审核环境",
  reviewerBoundarySub: "人工与 AI 审核者使用独立身份与签名密钥。",
  connectorBoundary: "服务器连接器",
  connectorBoundarySub: "校验本地策略、目录范围、签名和每次执行授权。",
  policyCaution:
    "被入侵的开发者设备仍可能使用预授权任务，以及有效授权的剩余次数。AI 判断不能证明操作一定安全。",
  githubTitle: "代码仓库权限与生产访问分离",
  githubNote:
    "移除直连 SSH 和 CI 生产凭据，仓库脚本不能自动成为服务器执行命令。",
  event: "事件",
  actor: "操作者",
  time: "时间",
  chain: "哈希链",
  auditRestricted: "需要团队审计员权限",
  auditRestrictedSub: "审计员可以查看当前团队的申请和事件历史。",
  noEvents: "暂无审计事件",
  auditCaution: "哈希链可检测局部修改；防止完整历史被重写需要独立不可变存储。",
  lightMode: "切换浅色主题",
  darkMode: "切换深色主题",
  language: "切换语言",
  reviewProgress: "审核进度",
  rootBoundary: "便捷操作，独立授权。",
  footer: "自托管访问控制",
  cliRequired: "请使用独立可信设备上的 CLI，或粘贴该设备生成的签名。",
  clipboardError: "无法使用剪贴板，请手动选择并复制。",
};

let memory = "zh|dark";
function subscribe(fn: () => void) {
  window.addEventListener("storage", fn);
  window.addEventListener("safeops-preferences", fn);
  return () => {
    window.removeEventListener("storage", fn);
    window.removeEventListener("safeops-preferences", fn);
  };
}
function snapshot() {
  try {
    const stored = localStorage.getItem("safeops-preferences");
    if (stored && /^(zh|en)\|(light|dark)$/.test(stored)) memory = stored;
  } catch {}
  return memory;
}
export function usePreferences() {
  const value = useSyncExternalStore(subscribe, snapshot, () => "zh|dark");
  const [locale, theme] = value.split("|");
  const update = (nextLocale: string, nextTheme: string) => {
    memory = `${nextLocale}|${nextTheme}`;
    try {
      localStorage.setItem("safeops-preferences", memory);
    } catch {}
    window.dispatchEvent(new Event("safeops-preferences"));
  };
  return {
    locale,
    theme,
    t: (key: Key) => (locale === "en" ? en : zh)[key],
    update,
    when: (ts: number) =>
      new Date(ts * 1000).toLocaleString(locale === "en" ? "en-US" : "zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      }),
  };
}
export function AppearanceControls() {
  const { locale, theme, t, update } = usePreferences();
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.lang = locale === "en" ? "en" : "zh-CN";
  }, [theme, locale]);
  return (
    <div className="appearance-controls">
      <button
        className="icon-button"
        aria-label={t(theme === "dark" ? "lightMode" : "darkMode")}
        title={t(theme === "dark" ? "lightMode" : "darkMode")}
        onClick={() => update(locale, theme === "dark" ? "light" : "dark")}
      >
        {theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}
      </button>
      <button
        className="language-button"
        aria-label={t("language")}
        onClick={() => update(locale === "zh" ? "en" : "zh", theme)}
      >
        <Languages size={16} />
        {locale === "zh" ? "English" : "中文"}
      </button>
    </div>
  );
}
export type TranslationKey = Key;
