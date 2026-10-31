"use client";

import { Select, SelectItem } from "@/components/select";

import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react";
import {
  ArrowDown,
  ArrowUpRight,
  BookOpen,
  Check,
  ChevronRight,
  Clipboard,
  CornerDownLeft,
  FileText,
  Folder,
  History,
  LoaderCircle,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Search,
  Server,
  ShieldCheck,
  Square,
  Terminal,
  X,
} from "lucide-react";
import {
  api,
  type Action,
  type DiagnosticOperation,
  type Project,
  type Request,
  type Target,
} from "@/lib/api";
import { usePreferences } from "@/lib/preferences";

const words = {
  zh: {
    servers: "服务器",
    search: "搜索服务器…",
    noServer: "当前身份没有可操作的服务器",
    noMatch: "没有匹配的服务器",
    open: "打开终端",
    close: "关闭终端标签",
    scope: "排障范围",
    mode: "授权方式",
    individual: "逐条审核",
    session: "会话授权",
    sessionMissing: "会话不可用，请重新选择授权方式",
    request: "申请授权",
    guide: "命令与范围",
    purpose: "工单 / 目的",
    purposeHint: "填写本次操作目的，可用于后续命令",
    prompt: "命令输入",
    send: "执行命令",
    hint: "输入命令，或按 Tab 查看可用命令",
    clear: "清屏",
    restore: "恢复记录",
    cleared: "已清屏。审计与执行记录仍保留。",
    welcome: "终端已就绪",
    demoShell: "本机演示：命令在当前用户下执行，演示身份不能用于生产。",
    welcomeHint: "Enter 提交 · Tab 补全 · ↑ ↓ 历史 · Ctrl+R 搜索历史",
    history: "命令历史",
    historySearch: "搜索历史命令…",
    noHistory: "暂无匹配的命令",
    edit: "放入输入行",
    copy: "复制输出",
    copied: "已复制",
    clipboard: "请选中输出，使用系统复制快捷键",
    details: "查看审核详情",
    latest: "最新输出",
    auto: "规则放行",
    ai: "AI 审核中",
    human: "等待人工审核",
    humanMode: "人工审核",
    pasteOne: "请一次粘贴一条命令，粘贴不会自动执行。",
    handoff: "已转人工",
    queued: "等待执行",
    running: "执行中",
    completed: "已完成",
    ready: "可用",
    failed: "执行失败",
    expired: "已过期",
    revoked: "已撤销",
    rejected: "已拒绝",
    cancel: "撤销操作",
    cancelSession: "撤销会话",
    toHuman: "转交人工",
    sent: "已提交",
    syncError: "已提交，结果暂未同步，请稍后刷新。",
    locked: "账户已锁定，暂停提交命令",
    waitSession: "会话暂不可执行；可先编辑下一条命令",
    remaining: "剩余",
    uses: "次",
    until: "截止",
    verified: "权限逐次检查",
    references: "可用命令",
    roots: "可访问目录",
    tasks: "已安装任务",
    plans: "变更计划",
    guideHint: "点击命令填入输入行，按 Enter 后才会提交。",
    shellHint:
      "支持通用命令、管道与重定向。安全诊断快速放行，风险或未知操作转人工。每条命令独立执行；切换目录请用 cd /path && 命令。暂不支持全屏交互程序。",
    sessionHint:
      "逐条审核只批准当前命令；切换到已批准会话，可在其目录、工具和剩余预算内连续操作。",
    helpHint: "help 查看命令 · clear 清屏 · Ctrl+C 清除输入 / 撤销最近活动操作",
    online: "在线",
    offline: "离线",
    recent: "操作记录",
    zoomIn: "增大字号",
    zoomOut: "减小字号",
    showServers: "展开服务器列表",
    hideServers: "收起服务器列表",
    sessionCommand: "排障会话",
    note: "审核说明",
    closeHelp: "关闭命令参考",
    closeHistory: "关闭命令历史",
    dismiss: "关闭提示",
    newRequest: "新建操作申请",
    cancelHint: "撤销会尽力中止运行，已产生的效果无法回滚。",
    noOutput: "无输出",
    waiting: "等待授权",
    processing: "正在提交…",
    saved: "草稿与历史仅保留在当前登录页面",
    next: "下一条命令",
    tabHint: "Tab 补全 · Esc 关闭",
    output: "终端输出",
    showHistory: "搜索命令历史",
    policy: "所有命令按服务器策略审核",
  },
  en: {
    servers: "Servers",
    search: "Find a server…",
    noServer: "No servers available for this identity",
    noMatch: "No matching servers",
    open: "Open terminal",
    close: "Close terminal tab",
    scope: "Diagnostic scope",
    mode: "Authorization",
    individual: "Review each command",
    session: "Session grant",
    sessionMissing: "Session unavailable. Choose an authorization mode.",
    request: "Request access",
    guide: "Commands & scope",
    purpose: "Ticket / purpose",
    purposeHint: "Purpose for subsequent operations",
    prompt: "Command input",
    send: "Run command",
    hint: "Enter a command, or press Tab to explore",
    clear: "Clear screen",
    restore: "Restore history",
    cleared: "Screen cleared. Audit and execution records are preserved.",
    welcome: "Terminal ready",
    demoShell:
      "Local demo: commands run as the current OS user. Demo identities are not for production.",
    welcomeHint:
      "Enter Submit · Tab Complete · ↑ ↓ History · Ctrl+R Search history",
    history: "Command history",
    historySearch: "Search command history…",
    noHistory: "No matching commands",
    edit: "Insert into prompt",
    copy: "Copy output",
    copied: "Copied",
    clipboard: "Select the output and use your system copy shortcut",
    details: "Review details",
    latest: "Latest output",
    auto: "Policy passed",
    ai: "AI reviewing",
    human: "Awaiting human review",
    humanMode: "Human review",
    pasteOne: "Paste one command at a time. Pasting never submits it.",
    handoff: "Handed to humans",
    queued: "Queued",
    running: "Running",
    completed: "Completed",
    ready: "Ready",
    failed: "Failed",
    expired: "Expired",
    revoked: "Revoked",
    rejected: "Rejected",
    cancel: "Revoke operation",
    cancelSession: "Revoke session",
    toHuman: "Request human review",
    sent: "Submitted",
    syncError: "Submitted. Results have not synced yet; refresh shortly.",
    locked: "Account locked. Command submission is paused.",
    waitSession: "Session is not ready. You can draft the next command.",
    remaining: "Remaining",
    uses: "uses",
    until: "Expires",
    verified: "Every operation checked",
    references: "Available commands",
    roots: "Accessible directories",
    tasks: "Installed tasks",
    plans: "Change plans",
    guideHint: "Click to insert. Only Enter or Run submits a command.",
    shellHint:
      "General commands, pipes and redirects are supported when the server executor is configured. Safe diagnostics pass quickly; risky or unknown operations require humans. Each command starts fresh; use cd /path && command. Fullscreen interactive programs are not supported.",
    sessionHint:
      "Individual review authorizes one command. An approved session allows continuous work within its tools, directories and remaining budget.",
    helpHint:
      "help Commands · clear Clear screen · Ctrl+C Clear input / revoke latest active operation",
    online: "Online",
    offline: "Offline",
    recent: "Activity",
    zoomIn: "Increase font size",
    zoomOut: "Decrease font size",
    showServers: "Show server list",
    hideServers: "Hide server list",
    sessionCommand: "Diagnostic session",
    note: "Review note",
    closeHelp: "Close command reference",
    closeHistory: "Close command history",
    dismiss: "Dismiss notice",
    newRequest: "New access request",
    cancelHint:
      "Revocation attempts to stop execution. Completed effects cannot be rolled back.",
    noOutput: "No output",
    waiting: "Awaiting authorization",
    processing: "Submitting…",
    saved: "Drafts and history stay in this login page only",
    next: "Next command",
    tabHint: "Tab Complete · Esc Dismiss",
    output: "Terminal output",
    showHistory: "Search command history",
    policy: "Server policy reviews every command",
  },
};
function useWords() {
  const p = usePreferences();
  return { ...p, c: words[p.locale === "en" ? "en" : "zh"] };
}
function plain(s: string) {
  return s
    .replace(/\x1b\][^\x07]*(?:\x07|\x1b\\)/g, "")
    .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]/g, "");
}
// The displayed command must be valid input when recalled from history.
function quote(s: string) {
  return /^[a-zA-Z0-9_./:-]+$/.test(s)
    ? s
    : "'" + s.replaceAll("'", "'\"'\"'") + "'";
}
function commandText(r: Request, op?: DiagnosticOperation) {
  if (op?.tool === "shell") return op.command || "";
  if (op) {
    const file = op.resource ? quote(`${op.resource}:${op.path}`) : "";
    if (op.tool === "grep") return `grep ${quote(op.query || "")} ${file}`;
    if (op.tool === "tail") return `tail -n ${op.lines || 50} ${file}`;
    return [op.tool, file].filter(Boolean).join(" ");
  }
  if (r.intent.kind === "plan")
    return `plan ${r.intent.action} ${r.intent.plan?.map((s) => s.id).join(" ")}`;
  if (r.intent.kind === "session") return `session ${r.intent.action}`;
  return `run ${r.intent.action}`;
}
type Entry = {
  key: string;
  request: Request;
  command: string;
  status: string;
  at: number;
  output?: string;
  exit?: number;
  sessionCommand?: boolean;
};
function entriesFor(requests: Request[]): Entry[] {
  return requests
    .flatMap((r) => {
      const entries: Entry[] = [];
      if (!r.executions?.length || r.intent.kind === "session")
        entries.push({
          key: r.intent.id,
          request: r,
          command: commandText(r, r.intent.operation),
          status: r.status,
          at: r.intent.created_at,
        });
      for (const execution of r.executions || [])
        entries.push({
          key: `${r.intent.id}:${execution.use}`,
          request: r,
          command: commandText(r, execution.operation || r.intent.operation),
          status:
            ["revoked", "expired"].includes(r.status) &&
            execution.status === "running"
              ? r.status
              : execution.status,
          at: execution.started_at,
          output: execution.output,
          exit: execution.exit_code,
          sessionCommand: r.intent.kind === "session",
        });
      return entries;
    })
    .sort((a, b) => a.at - b.at)
    .slice(-80);
}

export function TerminalWorkspace({
  token,
  targets,
  projects,
  requests,
  requester,
  refresh,
  openRequest,
  createRequest,
  heartbeats,
  now,
  locked,
}: {
  token: string;
  targets: Target[];
  projects: Project[];
  requests: Request[];
  requester: string;
  refresh: () => Promise<void>;
  openRequest: (id: string) => void;
  createRequest: (target: string, action?: string) => void;
  heartbeats: Record<string, number>;
  now: number;
  locked: boolean;
}) {
  const { c } = useWords();
  const available = targets.filter((t) => t.requesters.includes(requester));
  const [selected, setSelected] = useState("");
  const [opened, setOpened] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [showServers, setShowServers] = useState(false);
  const target = available.find((t) => t.id === selected) || available[0];
  const ids = Array.from(new Set([...opened, ...(target ? [target.id] : [])]));
  const tabs = ids
    .map((id) => available.find((t) => t.id === id))
    .filter((t): t is Target => !!t);
  function choose(id: string) {
    setOpened(Array.from(new Set([...ids, id])));
    setSelected(id);
  }
  if (!target)
    return (
      <div className="panel empty">
        <Terminal />
        <h3>{c.noServer}</h3>
      </div>
    );
  const matching = available.filter((t) =>
    `${t.name} ${t.id} ${projects.find((p) => p.id === t.project_id)?.name}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <div className={`command-workspace ${showServers ? "" : "servers-hidden"}`}>
      <aside className="terminal-servers" aria-label={c.servers}>
        <div className="terminal-section-title">
          <span>{c.servers}</span>
          <small>{available.length}</small>
        </div>
        <label className="server-search">
          <Search size={14} />
          <input
            aria-label={c.search}
            placeholder={c.search}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        {!matching.length && <p className="terminal-muted">{c.noMatch}</p>}
        {Array.from(new Set(matching.map((t) => t.project_id || ""))).map(
          (project) => (
            <div className="terminal-server-group" key={project}>
              <h3>
                <Folder size={13} />
                {projects.find((p) => p.id === project)?.name ||
                  project ||
                  c.servers}
              </h3>
              {matching
                .filter((t) => (t.project_id || "") === project)
                .map((t) => (
                  <button
                    key={t.id}
                    className={t.id === target.id ? "selected" : ""}
                    onClick={() => choose(t.id)}
                    aria-pressed={t.id === target.id}
                  >
                    <Server size={15} />
                    <span>
                      <strong>{t.name}</strong>
                      <small>{t.environment}</small>
                    </span>
                    <i
                      className={
                        !!heartbeats[t.id] && now - heartbeats[t.id] < 15
                          ? "online-dot"
                          : "offline-dot"
                      }
                      title={
                        !!heartbeats[t.id] && now - heartbeats[t.id] < 15
                          ? c.online
                          : c.offline
                      }
                    />
                  </button>
                ))}
            </div>
          ),
        )}
        <div className="server-rail-footer">
          <ShieldCheck size={16} />
          <span>
            {c.verified}
            <small>{c.saved}</small>
          </span>
        </div>
      </aside>
      <section className="terminal-workbench">
        <div className="terminal-tabbar">
          <button
            className="terminal-icon"
            title={showServers ? c.hideServers : c.showServers}
            aria-label={showServers ? c.hideServers : c.showServers}
            onClick={() => setShowServers(!showServers)}
          >
            {showServers ? (
              <PanelLeftClose size={17} />
            ) : (
              <PanelLeftOpen size={17} />
            )}
          </button>
          <div className="terminal-tabs" role="tablist" aria-label={c.servers}>
            {tabs.map((t) => (
              <div
                className={`terminal-tab ${t.id === target.id ? "active" : ""}`}
                key={t.id}
              >
                <button
                  role="tab"
                  aria-selected={t.id === target.id}
                  aria-controls={`console-${t.id}`}
                  id={`tab-${t.id}`}
                  onClick={() => choose(t.id)}
                >
                  <Terminal size={14} />
                  {t.name}
                </button>
                {tabs.length > 1 && (
                  <button
                    aria-label={`${c.close}: ${t.name}`}
                    title={c.close}
                    onClick={() => {
                      const next = ids.filter((id) => id !== t.id);
                      setOpened(next);
                      if (t.id === target.id) setSelected(next[0] || "");
                    }}
                  >
                    <X size={12} />
                  </button>
                )}
              </div>
            ))}
          </div>
          <button
            className="terminal-new"
            onClick={() =>
              createRequest(
                target.id,
                target.actions.find((a) => a.diagnostics)?.id,
              )
            }
            title={c.newRequest}
          >
            <Plus size={14} />
            <span>{c.request}</span>
          </button>
        </div>
        {tabs.map((t) => (
          <div
            className="terminal-tabpanel"
            role="tabpanel"
            aria-labelledby={`tab-${t.id}`}
            id={`console-${t.id}`}
            key={t.id}
            hidden={t.id !== target.id}
          >
            <ServerConsole
              token={token}
              target={t}
              requests={requests.filter(
                (r) =>
                  r.intent.requester === requester && r.intent.target === t.id,
              )}
              requester={requester}
              active={t.id === target.id}
              refresh={refresh}
              openRequest={openRequest}
              locked={locked}
              now={now}
            />
          </div>
        ))}
      </section>
    </div>
  );
}

function ServerConsole({
  token,
  target,
  requests,
  requester,
  active,
  refresh,
  openRequest,
  locked,
  now,
}: {
  token: string;
  target: Target;
  requests: Request[];
  requester: string;
  active: boolean;
  refresh: () => Promise<void>;
  openRequest: (id: string) => void;
  locked: boolean;
  now: number;
}) {
  const { c, locale } = useWords();
  const profiles = target.actions.filter((a) => a.diagnostics);
  const [profileID, setProfileID] = useState("");
  const profile =
    profiles.find((a) => a.id === profileID) ||
    profiles[0] ||
    target.actions[0];
  const [sessionID, setSessionID] = useState("");
  const [localRows, setLocalRows] = useState<Request[]>([]);
  const merged = [
    ...requests,
    ...localRows.filter(
      (r) => !requests.some((v) => v.intent.id === r.intent.id),
    ),
  ];
  const sessions = merged.filter(
    (r) => r.intent.kind === "session" && r.intent.action === profile?.id,
  );
  const grant = sessions.find((r) => r.intent.id === sessionID);
  const usable =
    !!grant &&
    grant.status === "ready" &&
    grant.intent.expires_at > now &&
    grant.used < grant.intent.max_uses;
  const [reason, setReason] = useState("");
  const [showReason, setShowReason] = useState(false);
  const [guide, setGuide] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyQuery, setHistoryQuery] = useState("");
  const [cleared, setCleared] = useState<Set<string>>(new Set());
  const [font, setFont] = useState(16);
  const [actionBusy, setActionBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [draft, setDraft] = useState("");
  const [submitted, setSubmitted] = useState<string[]>([]);
  const feed = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const follow = useRef(true);
  const allEntries = entriesFor(merged);
  const entries = allEntries.filter((e) => !cleared.has(e.key));
  const history = Array.from(
    new Set([
      ...submitted,
      ...allEntries
        .filter((e) => e.request.intent.kind !== "session" || e.sessionCommand)
        .map((e) => e.command)
        .reverse(),
    ]),
  );
  const latestActive = merged
    .filter(
      (r) =>
        ["pending", "approved", "running"].includes(r.status) &&
        (sessionID ? r.intent.id === sessionID : r.intent.kind !== "session"),
    )
    .sort((a, b) => b.intent.created_at - a.intent.created_at)[0];
  const disabled = locked || (!!sessionID && !usable);
  const version = entries
    .map((e) => `${e.key}:${e.status}:${e.output?.length || 0}`)
    .join("|");
  useLayoutEffect(() => {
    if (active && follow.current && feed.current)
      feed.current.scrollTop = feed.current.scrollHeight;
  }, [version, active]);
  useEffect(() => {
    const node = content.current;
    if (!node) return;
    const observer = new ResizeObserver(() => {
      if (follow.current && feed.current)
        feed.current.scrollTop = feed.current.scrollHeight;
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (active) input.current?.focus({ preventScroll: true });
  }, [active]);
  function insert(command: string) {
    setDraft(command);
    setHistoryOpen(false);
    follow.current = true;
    requestAnimationFrame(() => {
      input.current?.focus();
      if (feed.current) feed.current.scrollTop = feed.current.scrollHeight;
    });
  }
  function clear() {
    setCleared(
      new Set(
        allEntries
          .filter((e) => !["pending", "approved", "running"].includes(e.status))
          .map((e) => e.key),
      ),
    );
    setNotice("");
    follow.current = true;
    requestAnimationFrame(() => input.current?.focus());
  }
  async function act(r: Request, action: "revoke" | "handoff") {
    if (actionBusy) return;
    setActionBusy(r.intent.id);
    setNotice("");
    try {
      const next = await api<Request>(
        token,
        `/requests/${r.intent.id}/${action}`,
        {},
      );
      setLocalRows((v) => [
        next,
        ...v.filter((item) => item.intent.id !== next.intent.id),
      ]);
      await refresh();
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setActionBusy("");
      input.current?.focus({ preventScroll: true });
    }
  }
  const commands = commandOptions(target, profile, merged, !!sessionID);
  if (!profile) return <div className="terminal-welcome">{c.noServer}</div>;
  return (
    <div className="server-console">
      <div className="console-toolbar">
        <div className="console-selects">
          {profiles.length > 0 && (
            <label>
              <span>{c.scope}</span>
              <Select
                aria-label={c.scope}
                value={profile.id}
                onValueChange={(value) => {
                  setProfileID(value);
                  setSessionID("");
                }}
              >
                {profiles.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </Select>
            </label>
          )}
          <label>
            <span>{c.mode}</span>
            <Select
              aria-label={c.mode}
              value={sessionID || "individual"}
              onValueChange={(value) =>
                setSessionID(value === "individual" ? "" : value)
              }
            >
              <SelectItem value="individual">{c.individual}</SelectItem>
              {sessions
                .filter(
                  (r) =>
                    ["ready", "pending", "running", "approved"].includes(
                      r.status,
                    ) || r.intent.id === sessionID,
                )
                .map((r) => (
                  <SelectItem value={r.intent.id} key={r.intent.id}>
                    {c.session} · {r.intent.id.slice(0, 6)} ·{" "}
                    {r.status === "ready"
                      ? `${r.intent.max_uses - r.used} ${c.uses}`
                      : statusLabel(r.status, c)}
                  </SelectItem>
                ))}
            </Select>
          </label>
        </div>
        <div className="console-tools">
          <button
            className={`terminal-icon ${showReason ? "active" : ""}`}
            title={c.purpose}
            aria-label={c.purpose}
            aria-expanded={showReason}
            onClick={() => setShowReason(!showReason)}
          >
            <FileText size={16} />
          </button>
          <button
            className="terminal-icon"
            title={c.showHistory}
            aria-label={c.showHistory}
            onClick={() => setHistoryOpen(!historyOpen)}
          >
            <History size={16} />
          </button>
          <button
            className="terminal-icon"
            title={c.clear}
            aria-label={c.clear}
            onClick={clear}
          >
            <Square size={14} />
          </button>
          <button
            className={`terminal-icon ${guide ? "active" : ""}`}
            title={c.guide}
            aria-label={c.guide}
            aria-expanded={guide}
            onClick={() => setGuide(!guide)}
          >
            <BookOpen size={16} />
          </button>
        </div>
      </div>
      {showReason && (
        <label className="terminal-purpose">
          <span>{c.purpose}</span>
          <input
            aria-label={c.purposeHint}
            placeholder={c.purposeHint}
            maxLength={1000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </label>
      )}
      {!!sessionID && (
        <div className={`terminal-grant ${usable ? "" : "waiting"}`}>
          <ShieldCheck size={14} />
          {grant ? (
            <>
              <span>
                {c.session} · {c.remaining}{" "}
                {Math.max(0, grant.intent.max_uses - grant.used)} {c.uses} ·{" "}
                {c.until}{" "}
                {new Date(grant.intent.expires_at * 1000).toLocaleTimeString(
                  locale === "en" ? "en-US" : "zh-CN",
                  { hour12: false },
                )}
              </span>
              <button onClick={() => openRequest(grant.intent.id)}>
                {c.details}
                <ArrowUpRight size={12} />
              </button>
            </>
          ) : (
            c.sessionMissing
          )}
        </div>
      )}
      <div className={`terminal-body ${guide ? "with-guide" : ""}`}>
        <div
          className="console-output"
          ref={feed}
          aria-label={c.output}
          style={{ fontSize: font }}
          onScroll={() => {
            const el = feed.current;
            if (el)
              follow.current =
                el.scrollHeight - el.clientHeight - el.scrollTop < 80;
          }}
        >
          <div ref={content} className="console-transcript">
            <div className="terminal-welcome">
              <div>
                <Terminal size={17} />
                <strong>{target.name}</strong>
                <span>{target.environment}</span>
              </div>
              <p>{c.welcomeHint}</p>
              {profile.diagnostics?.shell?.demo && (
                <p className="terminal-demo-note">{c.demoShell}</p>
              )}
              {!entries.length && (
                <div className="terminal-quickstart">
                  {commands
                    .filter((command) =>
                      ["ls", "status", "help"].includes(command),
                    )
                    .map((command) => (
                      <button key={command} onClick={() => insert(command)}>
                        {command}
                        <CornerDownLeft size={12} />
                      </button>
                    ))}
                </div>
              )}
            </div>
            {cleared.size > 0 && (
              <div className="terminal-cleared">
                {c.cleared}
                <button onClick={() => setCleared(new Set())}>
                  {c.restore}
                </button>
              </div>
            )}
            {entries.map((entry) => (
              <TranscriptEntry
                key={entry.key}
                entry={entry}
                requester={requester}
                target={target.id}
                openRequest={openRequest}
                insert={insert}
                busy={actionBusy === entry.request.intent.id}
                act={act}
              />
            ))}
            {notice && (
              <div className="terminal-inline-notice" role="status">
                {plain(notice)}
                <button
                  className="terminal-icon"
                  aria-label={c.dismiss}
                  onClick={() => setNotice("")}
                >
                  <X size={12} />
                </button>
              </div>
            )}
            {locked && (
              <div className="terminal-inline-error" role="alert">
                {c.locked}
              </div>
            )}
            {!!sessionID && !usable && !locked && (
              <div className="terminal-inline-notice">{c.waitSession}</div>
            )}
            <TerminalPrompt
              input={input}
              draft={draft}
              setDraft={setDraft}
              token={token}
              target={target.id}
              profile={profile.id}
              session={sessionID}
              reason={reason}
              disabled={disabled}
              commands={commands}
              history={history}
              requester={requester}
              onClear={clear}
              onHelp={() => setGuide(true)}
              onHistory={() => setHistoryOpen(true)}
              onCancel={() => {
                if (latestActive) void act(latestActive, "revoke");
              }}
              onAccepted={async (result, command) => {
                follow.current = true;
                setSubmitted((v) =>
                  [command, ...v.filter((c) => c !== command)].slice(0, 100),
                );
                setLocalRows((v) =>
                  [
                    result,
                    ...v.filter((r) => r.intent.id !== result.intent.id),
                  ].slice(0, 80),
                );
                try {
                  await refresh();
                } catch {
                  setNotice(c.syncError);
                }
              }}
            />
            <div
              className="terminal-end-space"
              onClick={() => input.current?.focus()}
              aria-hidden="true"
            />
          </div>
        </div>
        {guide && (
          <aside className="console-guide">
            <div className="terminal-section-title">
              <span>{c.guide}</span>
              <button
                className="terminal-icon"
                aria-label={c.closeHelp}
                onClick={() => setGuide(false)}
              >
                <X size={15} />
              </button>
            </div>
            <p>{c.guideHint}</p>
            <h4>{c.references}</h4>
            {commands.slice(0, 18).map((command) => (
              <button
                className="console-example"
                key={command}
                onClick={() => insert(command)}
              >
                <code>{command}</code>
                <CornerDownLeft size={12} />
              </button>
            ))}
            <h4>{c.roots}</h4>
            {profile.diagnostics?.roots.map((root) => (
              <div className="console-root" key={root.id}>
                <code>{root.id}:</code>
                <span>{root.path}</span>
              </div>
            ))}
            <p>{c.sessionHint}</p>
            <p>{c.shellHint}</p>
          </aside>
        )}
        {historyOpen && (
          <div
            className="terminal-history"
            role="dialog"
            aria-label={c.history}
          >
            <div className="terminal-section-title">
              <span>
                <History size={14} />
                {c.history}
              </span>
              <button
                className="terminal-icon"
                aria-label={c.closeHistory}
                onClick={() => {
                  setHistoryOpen(false);
                  input.current?.focus();
                }}
              >
                <X size={15} />
              </button>
            </div>
            <input
              autoFocus
              aria-label={c.historySearch}
              placeholder={c.historySearch}
              value={historyQuery}
              onChange={(e) => setHistoryQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setHistoryOpen(false);
                  input.current?.focus();
                }
                if (e.key === "Enter") {
                  const found = history.find((h) =>
                    h.toLowerCase().includes(historyQuery.toLowerCase()),
                  );
                  if (found) insert(found);
                }
              }}
            />
            <div>
              {history
                .filter((h) =>
                  h.toLowerCase().includes(historyQuery.toLowerCase()),
                )
                .map((h, n) => (
                  <button key={h} onClick={() => insert(h)}>
                    <code>{h}</code>
                    <small>{n === 0 ? "↵" : ""}</small>
                  </button>
                ))}
              {!history.some((h) =>
                h.toLowerCase().includes(historyQuery.toLowerCase()),
              ) && <p>{c.noHistory}</p>}
            </div>
          </div>
        )}
      </div>
      <div className="terminal-statusbar">
        <span>
          <ShieldCheck size={13} />
          {sessionID ? c.session : c.policy}
        </span>
        <div>
          <button
            title={c.zoomOut}
            aria-label={c.zoomOut}
            disabled={font <= 11}
            onClick={() => setFont((v) => v - 1)}
          >
            A−
          </button>
          <button
            title={c.zoomIn}
            aria-label={c.zoomIn}
            disabled={font >= 18}
            onClick={() => setFont((v) => v + 1)}
          >
            A+
          </button>
          <button
            onClick={() => {
              follow.current = true;
              if (feed.current)
                feed.current.scrollTop = feed.current.scrollHeight;
              input.current?.focus({ preventScroll: true });
            }}
          >
            <ArrowDown size={13} />
            {c.latest}
          </button>
        </div>
      </div>
    </div>
  );
}
function statusLabel(status: string, c: typeof words.zh | typeof words.en) {
  return (
    (
      {
        pending: c.waiting,
        approved: c.queued,
        running: c.running,
        completed: c.completed,
        ready: c.ready,
        failed: c.failed,
        expired: c.expired,
        revoked: c.revoked,
        rejected: c.rejected,
      } as Record<string, string>
    )[status] || status
  );
}
function TranscriptEntry({
  entry: e,
  requester,
  target,
  openRequest,
  insert,
  busy,
  act,
}: {
  entry: Entry;
  requester: string;
  target: string;
  openRequest: (id: string) => void;
  insert: (c: string) => void;
  busy: boolean;
  act: (r: Request, a: "revoke" | "handoff") => Promise<void>;
}) {
  const { c, locale } = useWords();
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  const r = e.request;
  const pending = e.status === "pending";
  const running = e.status === "running" || e.status === "approved";
  const active = pending || running;
  const humanApproved =
    r.quorum > 0 &&
    r.approvals.filter((a) => a.kind !== "ai").length >= r.quorum;
  const mode = e.sessionCommand
    ? c.session
    : r.review.mode === "auto"
      ? c.auto
      : r.review.mode === "ai" && !humanApproved
        ? "AI"
        : c.humanMode;
  const status = pending
    ? r.needs_human
      ? c.handoff
      : r.review.mode === "ai"
        ? c.ai
        : c.human
    : statusLabel(e.status, c);
  return (
    <div className={`console-entry state-${e.status}`}>
      <div className="console-command">
        <span className="console-user">
          {requester}@{target}
        </span>
        <span className="console-dollar">$</span>
        <code>{e.command}</code>
        <time>
          {new Date(e.at * 1000).toLocaleTimeString(
            locale === "en" ? "en-US" : "zh-CN",
            { hour12: false, hour: "2-digit", minute: "2-digit" },
          )}
        </time>
        <button
          className="terminal-icon command-recall"
          title={c.edit}
          aria-label={`${c.edit}: ${e.command}`}
          onClick={() => insert(e.command)}
          disabled={r.intent.kind === "session" && !e.sessionCommand}
        >
          <CornerDownLeft size={13} />
        </button>
      </div>
      <div
        className={`console-result-meta ${pending && r.needs_human ? "needs-human" : ""}`}
        role={active ? "status" : undefined}
      >
        {active ? (
          <LoaderCircle
            size={12}
            className={
              running || (r.review.mode === "ai" && !r.needs_human)
                ? "terminal-spin"
                : ""
            }
          />
        ) : e.exit === 0 ? (
          <Check size={12} />
        ) : (
          <span className="terminal-status-dot" />
        )}
        <span>{status}</span>
        <span className="meta-divider">·</span>
        <span>{mode}</span>
        {e.exit !== undefined && (
          <span className={e.exit !== 0 ? "exit-error" : ""}>
            exit {e.exit}
          </span>
        )}
        <button title={c.details} onClick={() => openRequest(r.intent.id)}>
          <ArrowUpRight size={12} />
          {active ? c.details : ""}
        </button>
        {active && (
          <>
            <button
              disabled={busy}
              title={c.cancelHint}
              onClick={() => void act(r, "revoke")}
            >
              {r.intent.kind === "session" ? c.cancelSession : c.cancel}
            </button>
            {pending && r.review.mode === "ai" && !r.needs_human && (
              <button disabled={busy} onClick={() => void act(r, "handoff")}>
                {c.toHuman}
              </button>
            )}
          </>
        )}
      </div>
      {pending && r.needs_human && (
        <p className="console-handoff">
          {plain(r.assessments.at(-1)?.reason || c.human)}
        </p>
      )}
      {e.output !== undefined && (
        <div className="console-result">
          <pre>{plain(e.output).replace(/\n+$/, "") || c.noOutput}</pre>
          <button
            className="terminal-icon copy-output"
            title={copied ? c.copied : c.copy}
            aria-label={c.copy}
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(plain(e.output || ""));
                setCopied(true);
                setCopyError(false);
              } catch {
                setCopyError(true);
              }
            }}
          >
            {copied ? <Check size={13} /> : <Clipboard size={13} />}
          </button>
          {copyError && <small>{c.clipboard}</small>}
        </div>
      )}
    </div>
  );
}
function commandOptions(
  target: Target,
  profile: Action | undefined,
  requests: Request[],
  session: boolean,
) {
  const d = profile?.diagnostics;
  const root = d?.roots[0]?.id;
  const choices: string[] = [];
  for (const tool of d?.tools || []) {
    if (tool === "ls" || tool === "status") choices.push(tool);
    else if (root)
      choices.push(
        tool === "grep"
          ? `grep 'ERROR' ${root}:app.log`
          : tool === "tail"
            ? `tail -n 50 ${root}:app.log`
            : `cat ${root}:app.log`,
      );
  }
  for (const r of requests.slice(0, 20))
    for (const e of r.executions || []) {
      const op = e.operation || r.intent.operation;
      if (
        op?.tool !== "ls" ||
        r.intent.action !== profile?.id ||
        e.exit_code !== 0
      )
        continue;
      for (const filename of (e.output || "").split("\n").slice(0, 50)) {
        if (
          !filename ||
          filename.endsWith("/") ||
          /[\x00-\x1f`$;|&<>\\]/.test(filename)
        )
          continue;
        const path = quote(
          `${op.resource}:${op.path === "." ? "" : op.path + "/"}${filename}`,
        );
        for (const tool of d?.tools || [])
          if (["cat", "tail", "grep"].includes(tool))
            choices.push(
              tool === "grep"
                ? `grep 'ERROR' ${path}`
                : tool === "tail"
                  ? `tail -n 50 ${path}`
                  : `cat ${path}`,
            );
      }
    }
  if (!session)
    for (const a of target.actions) {
      if (a.plan)
        choices.push(
          `plan ${a.id} ${a.plan.steps
            .slice(0, a.plan.max_steps)
            .map((s) => s.id)
            .join(" ")}`,
        );
      else if (!a.diagnostics) choices.push(`run ${a.id}`);
    }
  return Array.from(new Set([...choices, "help", "clear"]));
}
function TerminalPrompt({
  input,
  draft,
  setDraft,
  token,
  target,
  profile,
  session,
  reason,
  disabled,
  commands,
  history,
  requester,
  onClear,
  onHelp,
  onHistory,
  onCancel,
  onAccepted,
}: {
  input: RefObject<HTMLInputElement | null>;
  draft: string;
  setDraft: (v: string) => void;
  token: string;
  target: string;
  profile: string;
  session: string;
  reason: string;
  disabled: boolean;
  commands: string[];
  history: string[];
  requester: string;
  onClear: () => void;
  onHelp: () => void;
  onHistory: () => void;
  onCancel: () => void;
  onAccepted: (r: Request, c: string) => Promise<void>;
}) {
  const { c } = useWords();
  const [busy, setBusy] = useState(false);
  const sending = useRef(false);
  const [error, setError] = useState("");
  const [suggest, setSuggest] = useState(false);
  const [choice, setChoice] = useState(0);
  const historyIndex = useRef(-1);
  const savedDraft = useRef("");
  const matches = commands.filter((v) => v.startsWith(draft)).slice(0, 8);
  function complete(index = choice) {
    const match = matches[index];
    if (match) {
      setDraft(match);
      setSuggest(false);
      input.current?.focus();
    }
  }
  return (
    <form
      className="console-prompt-form"
      onSubmit={async (e) => {
        e.preventDefault();
        if (sending.current || !draft.trim()) return;
        const command = draft.trim();
        if (command === "clear") {
          onClear();
          setDraft("");
          return;
        }
        if (command === "help") {
          onHelp();
          setDraft("");
          return;
        }
        if (disabled) return;
        sending.current = true;
        setBusy(true);
        setError("");
        setSuggest(false);
        try {
          const result = await api<Request>(token, "/terminal/command", {
            target,
            profile,
            command,
            reason,
            session: session || undefined,
          });
          setDraft("");
          historyIndex.current = -1;
          await onAccepted(result, command);
        } catch (e) {
          setError((e as Error).message);
        } finally {
          sending.current = false;
          setBusy(false);
          requestAnimationFrame(() => {
            if (input.current?.offsetParent !== null)
              input.current?.focus({ preventScroll: true });
          });
        }
      }}
    >
      <div className="console-input-line">
        <label htmlFor={`command-${target}`} className="console-user">
          {requester}@{target}
        </label>
        <span className="console-dollar">$</span>
        <input
          id={`command-${target}`}
          ref={input}
          aria-label={c.prompt}
          placeholder={c.hint}
          value={draft}
          readOnly={busy}
          maxLength={8192}
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          aria-controls={suggest ? `suggestions-${target}` : undefined}
          onChange={(e) => {
            setDraft(e.target.value);
            historyIndex.current = -1;
            setChoice(0);
            setSuggest(false);
          }}
          onPaste={(e) => {
            const value = e.clipboardData.getData("text");
            if (/[\r\n]/.test(value)) {
              e.preventDefault();
              setError(c.pasteOne);
            }
          }}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) {
              if (e.key === "Enter") e.preventDefault();
              return;
            }
            if (e.ctrlKey && e.key.toLowerCase() === "l") {
              e.preventDefault();
              onClear();
              return;
            }
            if (e.ctrlKey && e.key.toLowerCase() === "r") {
              e.preventDefault();
              onHistory();
              return;
            }
            if (
              e.ctrlKey &&
              e.key.toLowerCase() === "c" &&
              !window.getSelection()?.toString() &&
              e.currentTarget.selectionStart === e.currentTarget.selectionEnd
            ) {
              e.preventDefault();
              if (draft) setDraft("");
              else onCancel();
              return;
            }
            if (e.key === "Escape") {
              setSuggest(false);
              setError("");
              return;
            }
            if (e.key === "Tab" && !e.shiftKey) {
              e.preventDefault();
              if (matches.length === 1) complete(0);
              else if (suggest) complete();
              else {
                setChoice(0);
                setSuggest(true);
              }
              return;
            }
            if (suggest && ["ArrowUp", "ArrowDown"].includes(e.key)) {
              e.preventDefault();
              setChoice((v) =>
                Math.max(
                  0,
                  Math.min(
                    matches.length - 1,
                    v + (e.key === "ArrowUp" ? -1 : 1),
                  ),
                ),
              );
              return;
            }
            if (suggest && e.key === "Enter") {
              e.preventDefault();
              complete();
              return;
            }
            if (e.key === "ArrowUp") {
              e.preventDefault();
              if (historyIndex.current === -1) savedDraft.current = draft;
              historyIndex.current = Math.min(
                history.length - 1,
                historyIndex.current + 1,
              );
              if (historyIndex.current >= 0)
                setDraft(history[historyIndex.current]);
            }
            if (e.key === "ArrowDown") {
              e.preventDefault();
              if (historyIndex.current < 0) return;
              historyIndex.current = Math.max(-1, historyIndex.current - 1);
              setDraft(
                historyIndex.current === -1
                  ? savedDraft.current
                  : history[historyIndex.current],
              );
            }
          }}
        />
        <button
          className="terminal-run"
          title={c.send}
          aria-label={c.send}
          disabled={
            busy ||
            !draft.trim() ||
            (disabled && !["clear", "help"].includes(draft.trim()))
          }
        >
          {busy ? (
            <LoaderCircle size={15} className="terminal-spin" />
          ) : (
            <CornerDownLeft size={16} />
          )}
        </button>
      </div>
      {suggest && (
        <div className="terminal-completions" id={`suggestions-${target}`}>
          <small>{c.tabHint}</small>
          {matches.map((m, n) => (
            <button
              type="button"
              className={n === choice ? "selected" : ""}
              key={m}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => complete(n)}
            >
              <code>{m}</code>
              <ChevronRight size={12} />
            </button>
          ))}
        </div>
      )}
      {error && (
        <div className="terminal-inline-error" role="alert">
          {plain(error)}
        </div>
      )}
      <small className="terminal-input-hint">
        {busy ? c.processing : c.helpHint}
      </small>
    </form>
  );
}
