"use client";

import { useRef, useState } from "react";
import {
  ArrowDown,
  ArrowUp,
  Check,
  Play,
  Sparkles,
  Terminal,
  X,
} from "lucide-react";
import {
  api,
  operationLabel,
  type Action,
  type ChangeStep,
  type Request,
} from "@/lib/api";
import { usePreferences } from "@/lib/preferences";

const copy = {
  zh: {
    title: "受控终端",
    sub: "输入命令。看清授权。连续排障。",
    server: "目标服务器",
    profile: "排障范围",
    reason: "本次排障目的（可选）",
    placeholder: "例如：grep 'ERROR' logs:app.log",
    send: "提交命令",
    empty: "终端已就绪",
    emptySub: "选择服务器后输入命令；只有授权通过，连接器才会执行。",
    rules: "规则检查",
    ai: "AI 风险评估",
    human: "人工接管",
    flow: "简单命令直接放行；其余交给 AI，风险或不确定则转人工。",
    scope: "可访问目录",
    scopeNote: "目录由管理员配置，权限无法通过命令扩大。",
    supported: "命令参考",
    help: "引号支持文本参数。grep 使用字面搜索；不解释管道、重定向或 Shell 展开。",
    tasks: "已安装任务",
    plans: "变更计划",
    choose: "查看详情",
    pending: "AI 正在评估本次命令与近期操作…",
    handoff: "已转交人工审核",
    waiting: "等待独立人工审批",
    cancelled: "撤销",
    latest: "近期操作与输出",
    context: "AI 审核上下文",
    contextSub:
      "同一申请人、团队、项目、服务器；最近 24 小时，最多 10 条记录。输出经过脱敏和截断。",
    noContext: "没有可用的近期记录。",
    clipped: "部分上下文已截断",
    budget: "剩余次数",
    session: "已授权排障会话",
    sessionNote:
      "此会话内的范围已获批准。后续命令仍逐条校验目录、工具、时限和次数。",
    exact: "本次命令",
    plan: "按顺序执行的变更计划",
    privilege: "特权计划 · 需要两位独立人工签名和独立变更执行器",
    selectSteps: "选择步骤（点击加入，箭头调整顺序）",
    stepNote: "一次审批整个顺序；任何一步失败即停止。",
    up: "上移",
    down: "下移",
    remove: "移除",
    modeAuto: "规则放行",
    modeAI: "AI 审核",
    modeHuman: "人工审核",
    noServer: "当前身份没有可操作的服务器。",
    refresh: "已提交，等待授权与执行结果。",
    noOutput: "没有输出",
    selectedSession: "打开终端会话",
    history: "↑ ↓ 历史命令 · Ctrl+L 清空输入",
    expire: "会话截止",
    planRequired: "变更计划需要说明目的（至少 8 字节）。",
  },
  en: {
    title: "Controlled terminal",
    sub: "Enter commands. See authorization. Keep investigating.",
    server: "Target server",
    profile: "Diagnostic scope",
    reason: "Investigation purpose (optional)",
    placeholder: "Example: grep 'ERROR' logs:app.log",
    send: "Submit command",
    empty: "Terminal ready",
    emptySub:
      "Select a server and enter a command. The Connector executes only after authorization.",
    rules: "Policy checks",
    ai: "AI risk assessment",
    human: "Human handoff",
    flow: "Simple commands pass policy. Others go to AI, then to humans on risk or uncertainty.",
    scope: "Accessible directories",
    scopeNote: "Administrator-installed scope. Commands cannot expand it.",
    supported: "Command reference",
    help: "Quotes support text arguments. grep searches literal text. No pipes, redirects or shell expansion.",
    tasks: "Installed tasks",
    plans: "Change plans",
    choose: "View details",
    pending: "AI is assessing this command and recent activity…",
    handoff: "Handed to human reviewers",
    waiting: "Awaiting independent human approval",
    cancelled: "Revoke",
    latest: "Recent activity & output",
    context: "AI review context",
    contextSub:
      "Same requester, workspace, project and server. Last 24 hours, up to 10 records. Outputs are redacted and clipped.",
    noContext: "No recent records available.",
    clipped: "Some context was truncated",
    budget: "Remaining uses",
    session: "Authorized diagnostic session",
    sessionNote:
      "This session scope has been approved. Every command still undergoes directory, tool, expiry and use-budget checks.",
    exact: "Exact command",
    plan: "Ordered change plan",
    privilege:
      "Privileged plan · two independent human signatures and a separate change executor required",
    selectSteps: "Select steps (click to add; arrows reorder)",
    stepNote: "Approve the whole order once. Stop on the first failure.",
    up: "Move up",
    down: "Move down",
    remove: "Remove",
    modeAuto: "Policy passed",
    modeAI: "AI review",
    modeHuman: "Human review",
    noServer: "No servers available for this identity.",
    refresh: "Submitted; waiting for authorization and execution.",
    noOutput: "No output",
    selectedSession: "Open terminal session",
    history: "↑ ↓ Command history · Ctrl+L Clear input",
    expire: "Session expires",
    planRequired: "Change plans need a purpose (at least 8 bytes).",
  },
};
function useCopy() {
  const p = usePreferences();
  return { ...p, c: copy[p.locale === "en" ? "en" : "zh"] };
}
// Render output as inert text. Strip terminal control sequences; never interpret OSC links.
function plain(s: string) {
  return s
    .replace(/\x1b\][^\x07]*(?:\x07|\x1b\\)/g, "")
    .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]/g, "");
}

function CommandInput({
  token,
  target,
  profile,
  session,
  disabled,
  refresh,
}: {
  token: string;
  target: string;
  profile: string;
  session?: string;
  disabled?: boolean;
  refresh: () => Promise<void>;
}) {
  const { c } = useCopy();
  const [command, setCommand] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const history = useRef<string[]>([]);
  const index = useRef(0);
  return (
    <form
      className="terminal-input"
      onSubmit={async (e) => {
        e.preventDefault();
        if (busy || !command.trim()) return;
        setBusy(true);
        setError("");
        setSubmitted(false);
        try {
          await api<Request>(token, "/terminal/command", {
            target,
            profile,
            command,
            reason,
            session,
          });
          history.current.push(command);
          index.current = history.current.length;
          setCommand("");
          setSubmitted(true);
          await refresh();
        } catch (e) {
          setError((e as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      {!session && (
        <input
          aria-label={c.reason}
          placeholder={c.reason}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          maxLength={1000}
        />
      )}
      <div className="terminal-input-line">
        <span className="terminal-prompt">$</span>
        <input
          aria-label={c.send}
          placeholder={c.placeholder}
          autoComplete="off"
          spellCheck={false}
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          maxLength={2048}
          disabled={disabled || busy}
          onKeyDown={(e) => {
            if (e.key === "ArrowUp") {
              e.preventDefault();
              index.current = Math.max(0, index.current - 1);
              setCommand(history.current[index.current] || "");
            }
            if (e.key === "ArrowDown") {
              e.preventDefault();
              index.current = Math.min(
                history.current.length,
                index.current + 1,
              );
              setCommand(history.current[index.current] || "");
            }
            if (e.ctrlKey && e.key === "l") {
              e.preventDefault();
              setCommand("");
            }
          }}
        />
        <button
          className="primary"
          aria-label={c.send}
          disabled={disabled || busy || !command.trim()}
        >
          <Play size={15} />
          <span>{c.send}</span>
        </button>
      </div>
      <small>{c.history}</small>
      {error && (
        <div className="message error" role="alert">
          {error}
        </div>
      )}
      {submitted && !error && <small role="status">{c.refresh}</small>}
    </form>
  );
}
export function SessionConsole({
  request: r,
  token,
  refresh,
}: {
  request: Request;
  token: string;
  refresh: () => Promise<void>;
}) {
  const { c, when } = useCopy();
  return (
    <section className="session-console">
      <h4>
        <Terminal size={16} /> {c.session}
      </h4>
      <p>{c.sessionNote}</p>
      <div className="scope-root">
        <strong>
          {c.budget}: {r.intent.max_uses - r.used}
        </strong>
        <small>
          {c.expire}: {when(r.intent.expires_at)}
        </small>
      </div>
      <pre>
        {r.intent.scope?.roots
          .map((root) => `${root.id}: ${root.path}`)
          .join("\n")}
      </pre>
      <CommandInput
        token={token}
        target={r.intent.target}
        profile={r.intent.action}
        session={r.intent.id}
        disabled={r.status !== "ready"}
        refresh={refresh}
      />
    </section>
  );
}
export function WorkflowDetails({ request: r }: { request: Request }) {
  const { c, when } = useCopy();
  return (
    <>
      {r.intent.operation && (
        <section className="reason">
          <h4>{c.exact}</h4>
          <pre>{operationLabel(r.intent.operation)}</pre>
        </section>
      )}
      {r.intent.plan && (
        <section className="plan-preview">
          <h4>{c.plan}</h4>
          <p>{c.stepNote}</p>
          {r.intent.plan.map((s, n) => (
            <div key={s.id}>
              <strong>
                {n + 1}. {s.name}
              </strong>
              <pre>{s.argv.map((a) => JSON.stringify(a)).join(" ")}</pre>
              <small>{s.timeout_seconds}s</small>
            </div>
          ))}
        </section>
      )}
      {r.intent.scope && (
        <section className="reason">
          <h4>{c.scope}</h4>
          <pre>
            {r.intent.scope.roots
              .map((root) => `${root.id}: ${root.path}`)
              .join("\n")}
          </pre>
          <small>{r.intent.scope.tools.join(" · ")}</small>
        </section>
      )}
      {r.risk_context && (
        <details className="risk-context">
          <summary>
            <Sparkles size={15} /> {c.context} · {r.risk_context.records.length}
          </summary>
          <p>{c.contextSub}</p>
          <small>
            {when(r.risk_context.captured_at)}
            {r.risk_context.truncated && ` · ${c.clipped}`}
          </small>
          {r.risk_context.records.length === 0 && <p>{c.noContext}</p>}
          {r.risk_context.records.map((v, n) => (
            <div key={n}>
              <strong>
                {v.operation || v.action} · {v.status}
              </strong>
              <small>
                {when(v.at)}
                {v.exit_code !== undefined && ` · exit ${v.exit_code}`}
              </small>
              {v.output && <pre>{plain(v.output)}</pre>}
            </div>
          ))}
          {r.assessments
            ?.filter((a) => a.context_digest)
            .map((a, n) => (
              <code className="context-digest" key={n}>
                SHA256 {a.context_digest}
              </code>
            ))}
        </details>
      )}
    </>
  );
}
export function PlanSelector({
  action,
  selected,
  onChange,
}: {
  action: Action;
  selected: string[];
  onChange: (steps: string[]) => void;
}) {
  const { c } = useCopy();
  if (!action.plan) return null;
  const steps = selected
    .map((id) => action.plan!.steps.find((s) => s.id === id))
    .filter((s): s is ChangeStep => !!s);
  const move = (n: number, delta: number) => {
    const next = [...selected];
    [next[n], next[n + delta]] = [next[n + delta], next[n]];
    onChange(next);
  };
  return (
    <section className="plan-selector">
      <h4>{c.selectSteps}</h4>
      {action.plan.privileged && (
        <p className="callout warning">{c.privilege}</p>
      )}
      <div className="plan-choices">
        {action.plan.steps.map((s) => (
          <button
            className="secondary"
            type="button"
            disabled={
              selected.includes(s.id) ||
              selected.length >= action.plan!.max_steps
            }
            key={s.id}
            onClick={() => onChange([...selected, s.id])}
          >
            {selected.includes(s.id) ? <Check size={14} /> : "+"} {s.name}
          </button>
        ))}
      </div>
      {steps.map((s, n) => (
        <div className="plan-step" key={s.id}>
          <div>
            <strong>
              {n + 1}. {s.name}
            </strong>
            <pre>{s.argv.map((a) => JSON.stringify(a)).join(" ")}</pre>
          </div>
          <button
            type="button"
            className="icon-button"
            aria-label={c.up}
            disabled={n === 0}
            onClick={() => move(n, -1)}
          >
            <ArrowUp size={14} />
          </button>
          <button
            type="button"
            className="icon-button"
            aria-label={c.down}
            disabled={n === steps.length - 1}
            onClick={() => move(n, 1)}
          >
            <ArrowDown size={14} />
          </button>
          <button
            type="button"
            className="icon-button"
            aria-label={c.remove}
            onClick={() => onChange(selected.filter((id) => id !== s.id))}
          >
            <X size={14} />
          </button>
        </div>
      ))}
      <p>{c.stepNote}</p>
    </section>
  );
}
