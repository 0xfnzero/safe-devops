"use client";

import { Select, SelectItem } from "@/components/select";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity,
  ArrowRight,
  ArrowUpRight,
  Check,
  CheckCheck,
  ClipboardCheck,
  Clock3,
  Copy,
  FileKey2,
  Fingerprint,
  FolderKanban,
  GitBranch,
  KeyRound,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  Plus,
  RefreshCw,
  Server,
  Shield,
  ShieldCheck,
  Sparkles,
  Terminal,
  Users,
  X,
  XCircle,
} from "lucide-react";
import {
  operationLabel,
  api,
  demoSignature,
  type LockItem,
  type RiskScan,
  type Action,
  type Audit,
  type DemoUser,
  type Principal,
  type Project,
  type Request,
  type Target,
  type Workspace,
} from "@/lib/api";
import {
  AppearanceControls,
  usePreferences,
  type TranslationKey,
} from "@/lib/preferences";

import {
  SessionConsole,
  WorkflowDetails,
  PlanSelector,
} from "@/components/terminal-workspace";

import { TerminalWorkspace } from "@/components/command-terminal";
import "./terminal.css";

import { RiskCenter, LockBanner } from "@/components/risk-center";

const navigation = [
  { id: "terminal", icon: Terminal },
  { id: "overview", icon: LayoutDashboard },
  { id: "requests", icon: ClipboardCheck },
  { id: "assets", icon: Server },
  { id: "team", icon: Users },
  { id: "risk", icon: LockKeyhole },
  { id: "policy", icon: ShieldCheck },
  { id: "audit", icon: Activity },
] as const;
type Page = (typeof navigation)[number]["id"];
const statuses = new Set([
  "pending",
  "approved",
  "running",
  "ready",
  "completed",
  "failed",
  "revoked",
  "expired",
  "rejected",
]);
const approvalCount = (r: Request) =>
  Math.max(
    r.approvals.filter((a) => a.kind !== "ai").length,
    r.approvals.filter((a) => a.kind === "ai").length,
  );
function Badge({ status }: { status: string }) {
  const { t } = usePreferences();
  return (
    <span className={`badge ${status}`}>
      <i />
      {statuses.has(status) ? t(status as TranslationKey) : status}
    </span>
  );
}
function Brand() {
  return (
    <div className="wordmark">
      <span className="brand-mark">
        <Shield size={21} />
      </span>
      safe-devops
    </div>
  );
}

export default function Home() {
  const { t, when } = usePreferences();
  const session = useRef(0);
  const [deviceReady, setDeviceReady] = useState(false);
  const [deviceRequired, setDeviceRequired] = useState(false);
  const [token, setToken] = useState("");
  const [inputToken, setInputToken] = useState("");
  const [me, setMe] = useState<Principal | null>(null);
  const [demo, setDemo] = useState(false);
  const [demoUsers, setDemoUsers] = useState<DemoUser[]>([]);
  const [workspace, setWorkspace] = useState<Workspace>({ id: "", name: "" });
  const [projects, setProjects] = useState<Project[]>([]);
  const [members, setMembers] = useState<Principal[]>([]);
  const [targets, setTargets] = useState<Target[]>([]);
  const [heartbeats, setHeartbeats] = useState<Record<string, number>>({});
  const [locks, setLocks] = useState<LockItem[]>([]);
  const [scans, setScans] = useState<RiskScan[]>([]);
  const [requests, setRequests] = useState<Request[]>([]);
  const [events, setEvents] = useState<Audit[]>([]);
  const [page, setPage] = useState<Page>("overview");
  const [filter, setFilter] = useState("all");
  const [projectFilter, setProjectFilter] = useState("all");
  const [selected, setSelected] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [createDefaults, setCreateDefaults] = useState<{
    target: string;
    action?: string;
  }>();
  const [busy, setBusy] = useState(false);
  const [connected, setConnected] = useState(false);
  const [now, setNow] = useState(0);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<TranslationKey | null>(null);
  const [signature, setSignature] = useState("");
  const [rejection, setRejection] = useState("");
  useEffect(() => {
    fetch("/api/device/status")
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((d) => {
        setDeviceRequired(!!d.required);
        setDeviceReady(!!d.verified);
      })
      .catch(() => {
        setDeviceRequired(true);
        setDeviceReady(false);
      });
    fetch("/api/demo")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d?.users) setDemoUsers(d.users);
      })
      .catch(() => {});
  }, []);
  const refresh = useCallback(async () => {
    if (!token) return;
    const generation = session.current;
    const [catalog, list, lockData, scanData] = await Promise.all([
      api<{ targets: Target[]; heartbeats: Record<string, number> }>(
        token,
        "/catalog",
      ),
      api<{ requests: Request[] }>(token, "/requests"),
      api<{ locks: LockItem[] }>(token, "/account-locks"),
      api<{ scans: RiskScan[] }>(token, "/risk-scans"),
    ]);
    if (generation !== session.current) return;
    setTargets(catalog.targets);
    setHeartbeats(catalog.heartbeats);
    setRequests(list.requests);
    setLocks(lockData.locks);
    setScans(scanData.scans);
    setNow(Date.now() / 1000);
    setConnected(true);
    if (me?.roles.includes("auditor")) {
      const audit = await api<{ events: Audit[] }>(token, "/audit");
      if (generation === session.current) setEvents(audit.events);
    }
  }, [token, me]);
  useEffect(() => {
    if (!token) return;
    let active = true;
    const tick = () => {
      if (active)
        refresh().catch((e) => {
          if (active) {
            setConnected(false);
            setError(e.message);
          }
        });
    };
    tick();
    const timer = setInterval(tick, 4000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [refresh, token]);
  useEffect(() => {
    const close = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented) {
        setSelected(null);
        setShowCreate(false);
      }
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, []);
  async function login(value: string) {
    if (!deviceReady) {
      setError(t("deviceRequired"));
      return;
    }
    const generation = ++session.current;
    setBusy(true);
    setError("");
    try {
      const [identity, projectData, teamData] = await Promise.all([
        api<{ principal: Principal; demo: boolean; workspace: Workspace }>(
          value,
          "/me",
        ),
        api<{ projects: Project[] }>(value, "/projects"),
        api<{ members: Principal[] }>(value, "/team"),
      ]);
      if (generation !== session.current) return;
      setMe(identity.principal);
      if (
        new URLSearchParams(window.location.search).get("view") === "terminal"
      )
        setPage("terminal");
      setWorkspace(identity.workspace);
      setDemo(identity.demo);
      setToken(value);
      setProjects(projectData.projects);
      setMembers(teamData.members);
      setTargets([]);
      setRequests([]);
      setLocks([]);
      setScans([]);
      setEvents([]);
      setSelected(null);
      setNotice(null);
      setInputToken("");
      setProjectFilter("all");
      setConnected(true);
    } catch (e) {
      if (generation === session.current) setError((e as Error).message);
    } finally {
      if (generation === session.current) setBusy(false);
    }
  }
  async function mutate(fn: () => Promise<unknown>, message: TranslationKey) {
    const generation = session.current;
    setBusy(true);
    setError("");
    setNotice(null);
    try {
      await fn();
      if (generation !== session.current) return;
      await refresh();
      setNotice(message);
    } catch (e) {
      if (generation === session.current) setError((e as Error).message);
    } finally {
      if (generation === session.current) setBusy(false);
    }
  }
  const targetName = (id: string) =>
    targets.find((x) => x.id === id)?.name || id;
  const projectName = (id?: string) =>
    projects.find((p) => p.id === id)?.name || t("defaultProject");
  const actionFor = (r: Request) =>
    targets
      .find((x) => x.id === r.intent.target)
      ?.actions.find((x) => x.id === r.intent.action);
  const pending = requests.filter((r) => r.status === "pending").length;
  const activeGrants = requests.filter((r) =>
    ["approved", "ready", "running"].includes(r.status),
  ).length;
  const online = targets.filter(
    (x) => heartbeats[x.id] && now - heartbeats[x.id] < 15,
  ).length;
  const current = requests.find((r) => r.intent.id === selected);
  const canRequest = me?.roles.includes("requester");
  const humanReviewer = !!(
    me &&
    current &&
    me.kind !== "ai" &&
    me.roles.includes("approver") &&
    me.id !== current.intent.requester &&
    (actionFor(current)?.approvers || []).includes(me.id)
  );
  const openRequest = (id: string) => {
    setSelected(id);
    setSignature("");
    setRejection("");
  };
  async function approve(r: Request) {
    const user = demo ? demoUsers.find((u) => u.id === me?.id) : undefined;
    const sig = user?.seed
      ? await demoSignature(user.seed, r.payload)
      : signature.trim();
    if (!sig) throw new Error(t("cliRequired"));
    return api(token, `/requests/${r.intent.id}/approve`, { signature: sig });
  }
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice("copied");
    } catch {
      setError(t("clipboardError"));
    }
  }
  const statusText = (value: string) =>
    statuses.has(value) ? t(value as TranslationKey) : value;

  if (!me)
    return (
      <div className="login-page">
        <section className="login-art">
          <Brand />
          <div className="login-story">
            <span className="eyebrow">SCOPED ACCESS / INDEPENDENT TRUST</span>
            <h1>{t("welcome")}</h1>
            <p>{t("welcomeSub")}</p>
            <div className="login-boundaries">
              {[
                { icon: Users, key: "workspace" },
                { icon: FolderKanban, key: "project" },
                { icon: Server, key: "assets" },
              ].map(({ icon: Icon, key }) => (
                <div key={key}>
                  <Icon size={22} />
                  <span>{t(key as TranslationKey)}</span>
                </div>
              ))}
            </div>
            <div className="login-trust">
              <ShieldCheck size={17} />
              {t("rootBoundary")}
            </div>
          </div>
          <small>Go + Next.js · SQLite / PostgreSQL · MIT</small>
        </section>
        <main className="login-form">
          <div className="login-preferences">
            <AppearanceControls />
          </div>
          <div className="login-card">
            <span className="mini-logo">
              <KeyRound size={25} />
            </span>
            <h2>{t("loginTitle")}</h2>
            <p className="muted">{t("loginSub")}</p>
            {error && (
              <div role="alert" className="message error">
                {error}
              </div>
            )}
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void login(inputToken.trim());
              }}
            >
              {!deviceReady && (
                <div className="callout warning" role="status">
                  {t(deviceRequired ? "deviceRequired" : "deviceChecking")}
                </div>
              )}
              <label htmlFor="token">{t("credential")}</label>
              <input
                id="token"
                disabled={!deviceReady}
                type="password"
                autoComplete="off"
                value={inputToken}
                onChange={(e) => setInputToken(e.target.value)}
                required
              />
              <button
                className="primary full"
                disabled={busy || !deviceReady || !inputToken.trim()}
              >
                {t("signIn")}
                <ArrowRight size={16} />
              </button>
            </form>
            {demoUsers.length > 0 && (
              <div className="demo-login">
                <div className="divider">{t("demoIdentities")}</div>
                <div className="demo-grid">
                  {demoUsers.map((u) => (
                    <button
                      key={u.id}
                      disabled={busy}
                      onClick={() => void login(u.token)}
                    >
                      <span className="avatar">{u.id[0].toUpperCase()}</span>
                      <span>
                        {u.name.split(" · ")[0]}
                        <small>{t((u.role + "Role") as TranslationKey)}</small>
                      </span>
                      <ArrowUpRight size={14} />
                    </button>
                  ))}
                </div>
              </div>
            )}
            <p className="login-foot">
              <LockKeyhole size={14} />
              {t("sessionOnly")}
            </p>
          </div>
        </main>
      </div>
    );

  return (
    <div className={`app-shell ${page === "terminal" ? "terminal-view" : ""}`}>
      <aside className="sidebar">
        <Brand />
        <div className="workspace-label">
          <span className="workspace-icon">{workspace.name?.[0] || "S"}</span>
          <div>
            <strong>{workspace.name || t("workspace")}</strong>
            <small>{workspace.id}</small>
          </div>
        </div>
        <p className="nav-label">{t("operations")}</p>
        <nav>
          {navigation.map((n, index) => (
            <div key={n.id}>
              {index === 4 && (
                <p className="nav-label security-label">{t("security")}</p>
              )}
              <button
                className={page === n.id ? "active" : ""}
                aria-label={t(n.id)}
                title={t(n.id)}
                onClick={() => {
                  setPage(n.id);
                  setSelected(null);
                }}
              >
                <n.icon size={18} />
                <span className="nav-text">{t(n.id)}</span>
                {n.id === "requests" && pending > 0 && (
                  <span className="nav-count">{pending}</span>
                )}
              </button>
            </div>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="boundary-note">
            <ShieldCheck size={19} />
            <div>
              <strong>{t("rootBoundary")}</strong>
              <small>{t("footer")}</small>
            </div>
          </div>
          <span className="version">
            v0.4.1 <span>OPEN SOURCE</span>
          </span>
        </div>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <div className="breadcrumb">
            <span>{workspace.name || t("workspace")}</span>
            <span>/</span>
            <strong>{t(page)}</strong>
          </div>
          <div className="top-actions">
            <span className={`connected ${connected ? "" : "disconnected"}`}>
              <i />
              {t(connected ? "connected" : "disconnected")}
            </span>
            <AppearanceControls />
            {demo && (
              <Select
                aria-label={t("switchIdentity")}
                value={me.id}
                disabled={busy}
                onValueChange={(value) => {
                  const u = demoUsers.find((u) => u.id === value);
                  if (u) void login(u.token);
                }}
              >
                {demoUsers.map((u) => (
                  <SelectItem key={u.id} value={u.id}>
                    {u.name.split(" · ")[0]}
                  </SelectItem>
                ))}
              </Select>
            )}
            <span className="avatar">{me.id[0].toUpperCase()}</span>
            <button
              className="icon-button"
              disabled={busy}
              aria-label={t("signOut")}
              onClick={() => {
                session.current++;
                setMe(null);
                setToken("");
                setRequests([]);
                setLocks([]);
                setScans([]);
                setTargets([]);
                setEvents([]);
                setSelected(null);
              }}
            >
              <LogOut size={17} />
            </button>
          </div>
        </header>
        {demo && (
          <div className="demo-banner">
            <span>{t("demo")}</span>
            <p>{t("demoNote")}</p>
          </div>
        )}
        <main className="content">
          {error && (
            <div className="message error" role="alert">
              {error}
              <button aria-label={t("close")} onClick={() => setError("")}>
                <X size={16} />
              </button>
            </div>
          )}
          {notice && (
            <div className="message success" role="status">
              {t(notice)}
              <button aria-label={t("close")} onClick={() => setNotice(null)}>
                <X size={16} />
              </button>
            </div>
          )}
          {page !== "terminal" &&
            requests
              .filter(
                (r) =>
                  r.intent.requester === me.id &&
                  r.status === "pending" &&
                  r.needs_human,
              )
              .map((r) => (
                <div
                  className="message handoff-notice"
                  role="status"
                  key={r.intent.id}
                >
                  <Sparkles size={18} />
                  <div>
                    <strong>{t("handoffNotice")}</strong>
                    <p>{r.assessments.at(-1)?.reason || t("humanWarning")}</p>
                  </div>
                  <button
                    className="secondary"
                    onClick={() => openRequest(r.intent.id)}
                  >
                    {t("detail")}
                  </button>
                </div>
              ))}
          <LockBanner items={locks} requester={me.id} now={now} />
          <div className="page-heading">
            <div>
              <h1>{page === "overview" ? t("overviewTitle") : t(page)}</h1>
              <p>
                {t(
                  (
                    {
                      risk: "riskSub",
                      terminal: "terminalSub",
                      overview: "overviewSub",
                      requests: "requestSub",
                      assets: "assetsSub",
                      team: "teamSub",
                      policy: "policySub",
                      audit: "auditSub",
                    } as const
                  )[page],
                )}
              </p>
            </div>
            <div className="heading-actions">
              <button
                className="secondary icon-button"
                aria-label={t("refresh")}
                disabled={busy}
                onClick={() => void refresh().catch((e) => setError(e.message))}
              >
                <RefreshCw size={16} />
              </button>
              {canRequest && (
                <button className="primary" onClick={() => setShowCreate(true)}>
                  <Plus size={16} />
                  {t("create")}
                </button>
              )}
            </div>
          </div>
          {page === "risk" && (
            <RiskCenter
              items={locks}
              scans={scans}
              token={token}
              demoUser={
                demo ? demoUsers.find((u) => u.id === me.id) : undefined
              }
              now={now}
              refresh={refresh}
            />
          )}
          {page === "terminal" && (
            <TerminalWorkspace
              key={workspace.id + ":" + me.id}
              token={token}
              targets={targets}
              projects={projects}
              requests={requests}
              requester={me.id}
              heartbeats={heartbeats}
              now={now}
              locked={locks.some(
                ({ lock }) =>
                  lock.requester === me.id &&
                  !lock.released_at &&
                  lock.expires_at > now,
              )}
              createRequest={(target, action) => {
                setCreateDefaults({ target, action });
                setShowCreate(true);
              }}
              refresh={refresh}
              openRequest={openRequest}
            />
          )}
          {page === "overview" && (
            <>
              <div className="metrics">
                {[
                  {
                    label: "managed",
                    value: targets.length,
                    note: `${online} ${t("connectorOnline")}`,
                    icon: Server,
                  },
                  {
                    label: "pendingMetric",
                    value: pending,
                    note: t("noExecution"),
                    icon: Clock3,
                  },
                  {
                    label: "activeMetric",
                    value: activeGrants,
                    note: t("boundedWindow"),
                    icon: KeyRound,
                  },
                  {
                    label: "historyMetric",
                    value: requests.length,
                    note: t("recent200"),
                    icon: ClipboardCheck,
                  },
                ].map(({ label, value, note, icon: Icon }) => (
                  <div className="metric" key={label}>
                    <div>
                      <span>{t(label as TranslationKey)}</span>
                      <Icon size={18} />
                    </div>
                    <strong>{String(value).padStart(2, "0")}</strong>
                    <small>{note}</small>
                  </div>
                ))}
              </div>
              <div className="project-overview">
                {projects.map((p) => (
                  <button
                    className="project-summary"
                    key={p.id}
                    onClick={() => {
                      setProjectFilter(p.id);
                      setPage("assets");
                    }}
                  >
                    <span className="project-summary-icon">
                      <FolderKanban size={20} />
                    </span>
                    <span>
                      <strong>{p.name}</strong>
                      <small>
                        {targets.filter((t) => t.project_id === p.id).length}{" "}
                        {t("servers")}
                        <span>·</span>
                        {p.members.length} {t("members")}
                      </small>
                    </span>
                    <ArrowUpRight size={17} />
                  </button>
                ))}
              </div>
            </>
          )}
          {(page === "overview" || page === "requests") && (
            <section className="panel request-panel">
              <div className="panel-heading">
                <div>
                  <h2>
                    {t(page === "overview" ? "recent" : "requests")}
                    <span className="count">{requests.length}</span>
                  </h2>
                  <p>{t("requestTableSub")}</p>
                </div>
                <span className="live-label">
                  <i />
                  {t("updated")}
                </span>
              </div>
              <div className="filters">
                {["all", "pending", "ready", "running", "completed"].map(
                  (id) => (
                    <button
                      key={id}
                      className={filter === id ? "selected" : ""}
                      onClick={() => setFilter(id)}
                    >
                      {t(id as TranslationKey)}
                      {id === "pending" && pending > 0 && (
                        <span>{pending}</span>
                      )}
                    </button>
                  ),
                )}
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>{t("taskTarget")}</th>
                      <th>{t("requester")}</th>
                      <th>{t("review")}</th>
                      <th>{t("status")}</th>
                      <th>{t("created")}</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {requests
                      .filter((r) => filter === "all" || r.status === filter)
                      .map((r) => (
                        <tr
                          key={r.intent.id}
                          onClick={() => openRequest(r.intent.id)}
                        >
                          <td>
                            <button
                              className="request-link"
                              onClick={() => openRequest(r.intent.id)}
                            >
                              <span className="table-icon">
                                <Terminal size={17} />
                              </span>
                              <span>
                                {actionFor(r)?.name || r.intent.action}
                                <small>
                                  {projectName(r.intent.project_id)} /{" "}
                                  {targetName(r.intent.target)}
                                </small>
                              </span>
                            </button>
                          </td>
                          <td>
                            <span className="person-dot">
                              {r.intent.requester[0].toUpperCase()}
                            </span>
                            {r.intent.requester}
                          </td>
                          <td>
                            <span className="review-mode">
                              {t(
                                r.needs_human && r.status === "pending"
                                  ? "humanNeeded"
                                  : r.review.mode,
                              )}
                            </span>
                            <small className="muted">
                              {r.review.mode === "auto"
                                ? t("preauthorized")
                                : `${approvalCount(r)} / ${r.quorum}`}
                            </small>
                          </td>
                          <td>
                            <Badge status={r.status} />
                          </td>
                          <td className="time-cell">
                            {when(r.intent.created_at)}
                          </td>
                          <td>
                            <ArrowUpRight size={15} />
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
                {requests.filter((r) => filter === "all" || r.status === filter)
                  .length === 0 && (
                  <div className="empty">
                    <ClipboardCheck size={32} />
                    <h3>{t("noRequests")}</h3>
                    <p>{t("noRequestsSub")}</p>
                    {canRequest && (
                      <button
                        className="secondary"
                        onClick={() => setShowCreate(true)}
                      >
                        <Plus size={15} />
                        {t("create")}
                      </button>
                    )}
                  </div>
                )}
              </div>
              <div className="panel-footer">
                <LockKeyhole size={13} />
                {t("boundedWindow")}
                <span>{t("recent200")}</span>
              </div>
            </section>
          )}
          {page === "overview" && (
            <div className="bottom-grid">
              <section className="panel connector-panel">
                <div className="panel-heading">
                  <h2>
                    <Server size={17} />
                    {t("connectorState")}
                  </h2>
                  <button
                    className="text-button"
                    onClick={() => setPage("assets")}
                  >
                    {t("assets")}
                    <ArrowRight size={14} />
                  </button>
                </div>
                {targets.map((target) => (
                  <div className="agent-row" key={target.id}>
                    <span className="table-icon">
                      <Server size={18} />
                    </span>
                    <div>
                      <strong>{target.name}</strong>
                      <small>
                        {projectName(target.project_id)} · {target.environment}
                      </small>
                    </div>
                    <span
                      className={`online ${now - (heartbeats[target.id] || 0) < 15 ? "yes" : ""}`}
                    >
                      <i />
                      {t(
                        now - (heartbeats[target.id] || 0) < 15
                          ? "online"
                          : "offline",
                      )}
                    </span>
                  </div>
                ))}
              </section>
              <section className="panel quick-policy">
                <div className="panel-heading">
                  <h2>
                    <ShieldCheck size={17} />
                    {t("policy")}
                  </h2>
                  <button
                    className="text-button"
                    aria-label={t("policy")}
                    onClick={() => setPage("policy")}
                  >
                    <ArrowUpRight size={16} />
                  </button>
                </div>
                {[
                  { key: "auto", icon: Check, description: "policyAutoTitle" },
                  { key: "ai", icon: Sparkles, description: "policyAITitle" },
                  {
                    key: "manual",
                    icon: FileKey2,
                    description: "policyHumanTitle",
                  },
                ].map(({ key, icon: Icon, description }) => (
                  <div className="policy-short" key={key}>
                    <Icon size={17} />
                    <strong>{t(key as TranslationKey)}</strong>
                    <span>{t(description as TranslationKey)}</span>
                  </div>
                ))}
              </section>
            </div>
          )}
          {page === "assets" && (
            <>
              <div className="project-toolbar">
                <label htmlFor="project-filter">{t("project")}</label>
                <Select
                  id="project-filter"
                  value={projectFilter}
                  onValueChange={(value) => setProjectFilter(value)}
                >
                  <SelectItem value="all">{t("allProjects")}</SelectItem>
                  {projects.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </Select>
                <span>
                  {
                    targets.filter(
                      (x) =>
                        projectFilter === "all" ||
                        x.project_id === projectFilter,
                    ).length
                  }{" "}
                  {t("servers")}
                </span>
              </div>
              <div className="asset-grid">
                {targets
                  .filter(
                    (x) =>
                      projectFilter === "all" || x.project_id === projectFilter,
                  )
                  .map((target) => (
                    <section className="panel asset-card" key={target.id}>
                      <div className="asset-title">
                        <span className="asset-icon">
                          <Server size={23} />
                        </span>
                        <span
                          className={`online ${now - (heartbeats[target.id] || 0) < 15 ? "yes" : ""}`}
                        >
                          <i />
                          {t(
                            now - (heartbeats[target.id] || 0) < 15
                              ? "online"
                              : "offline",
                          )}
                        </span>
                      </div>
                      <h2>{target.name}</h2>
                      <p className="muted mono">{target.id}</p>
                      <span className="project-chip">
                        {projectName(target.project_id)}
                      </span>
                      <div className="asset-meta">
                        <span>
                          {t("environment")}
                          <strong>{target.environment}</strong>
                        </span>
                        <span>
                          {t("lastSeen")}
                          <strong>
                            {heartbeats[target.id]
                              ? when(heartbeats[target.id])
                              : t("never")}
                          </strong>
                        </span>
                      </div>
                      <h4>{t("fixedTasks")}</h4>
                      {target.actions.map((a) => (
                        <div key={a.id} className="runbook">
                          <Terminal size={16} />
                          <div>
                            <strong>{a.name}</strong>
                            <small>{a.description}</small>
                          </div>
                          <span className={`mode-tag ${a.review.mode}`}>
                            {t(a.review.mode)}
                          </span>
                        </div>
                      ))}
                    </section>
                  ))}
              </div>
            </>
          )}
          {page === "team" && (
            <TeamView
              workspace={workspace}
              projects={projects}
              members={members}
              targets={targets}
            />
          )}
          {page === "policy" && <PolicyView />}
          {page === "audit" && (
            <section className="panel">
              <div className="panel-heading">
                <div>
                  <h2>{t("audit")}</h2>
                  <p>{t("auditSub")}</p>
                </div>
                <span className="pill">SHA-256</span>
              </div>
              {!me.roles.includes("auditor") ? (
                <div className="empty">
                  <FileKey2 size={32} />
                  <h3>{t("auditRestricted")}</h3>
                  <p>{t("auditRestrictedSub")}</p>
                </div>
              ) : (
                <>
                  <div className="table-scroll">
                    <table>
                      <thead>
                        <tr>
                          <th>#</th>
                          <th>{t("event")}</th>
                          <th>{t("actor")}</th>
                          <th>{t("time")}</th>
                          <th>{t("chain")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {[...events].reverse().map((e) => (
                          <tr key={e.sequence}>
                            <td>{e.sequence}</td>
                            <td>
                              <strong className="audit-event">{e.event}</strong>
                              <small className="audit-detail">{e.detail}</small>
                            </td>
                            <td>{e.actor}</td>
                            <td className="time-cell">{when(e.at)}</td>
                            <td className="mono muted">
                              {e.hash.slice(0, 16)}…
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    {events.length === 0 && (
                      <div className="empty">{t("noEvents")}</div>
                    )}
                  </div>
                  <div className="panel-footer">{t("auditCaution")}</div>
                </>
              )}
            </section>
          )}
          <footer>
            <span>
              safe-devops <b>·</b> {t("footer")}
            </span>
            <span>v0.4.1 / MIT</span>
          </footer>
        </main>
      </div>
      {showCreate && (
        <CreateDialog
          initialTarget={
            page === "terminal" ? createDefaults?.target : undefined
          }
          initialAction={
            page === "terminal" ? createDefaults?.action : undefined
          }
          targets={targets.filter((x) => x.requesters.includes(me.id))}
          projects={projects}
          busy={busy}
          onClose={() => setShowCreate(false)}
          onCreate={(body) =>
            mutate(async () => {
              const r = await api<Request>(token, "/requests", body);
              openRequest(r.intent.id);
              setShowCreate(false);
            }, "grantSubmitted")
          }
        />
      )}
      {current && (
        <div className="drawer-backdrop" onClick={() => setSelected(null)}>
          <aside
            className="drawer"
            role="dialog"
            aria-modal="true"
            aria-label={t("detail")}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="drawer-top">
              <span className="eyebrow">{t("detail")}</span>
              <button
                className="icon-button"
                aria-label={t("close")}
                onClick={() => setSelected(null)}
              >
                <X size={20} />
              </button>
            </div>
            <h2>{actionFor(current)?.name || current.intent.action}</h2>
            <div className="drawer-subtitle">
              <Badge status={current.status} />
              <code>{current.intent.id.slice(0, 12)}</code>
            </div>
            <dl className="details">
              <dt>{t("scope")}</dt>
              <dd>
                {workspace.name} / {projectName(current.intent.project_id)}
              </dd>
              <dt>{t("target")}</dt>
              <dd>{targetName(current.intent.target)}</dd>
              <dt>{t("requester")}</dt>
              <dd>{current.intent.requester}</dd>
              <dt>{t("expiresAt")}</dt>
              <dd>{when(current.intent.expires_at)}</dd>
              <dt>{t("review")}</dt>
              <dd>
                {t(current.review.mode)} · {t(current.review.risk)}
              </dd>
              <dt>{t("executionCount")}</dt>
              <dd>
                {current.used} / {current.intent.max_uses} {t("uses")}
              </dd>
            </dl>
            <section className="reason">
              <h4>{t("reason")}</h4>
              <p>{current.intent.reason}</p>
            </section>
            <WorkflowDetails request={current} />
            {current.intent.kind === "session" &&
              current.intent.requester === me.id && (
                <SessionConsole
                  request={current}
                  token={token}
                  refresh={refresh}
                />
              )}
            <div className="fingerprint">
              <div>
                <Fingerprint size={17} />
                <strong>{t("fingerprint")}</strong>
                <button
                  className="icon-button"
                  aria-label={t("copy")}
                  onClick={() => void copy(current.fingerprint)}
                >
                  <Copy size={14} />
                </button>
              </div>
              <code>{current.fingerprint}</code>
              <p>{t("signatureScope")}</p>
            </div>
            <h4 className="section-label">{t("approvals")}</h4>
            {current.approvals.length ? (
              current.approvals.map((a) => (
                <div className="approval-person" key={a.subject}>
                  <CheckCheck size={17} />
                  <strong>
                    {a.subject}
                    <small>
                      {t(a.kind === "ai" ? "aiIdentity" : "humanIdentity")}
                    </small>
                  </strong>
                  <span>{when(a.at)}</span>
                </div>
              ))
            ) : (
              <p className="muted small">
                {t(
                  current.review.mode === "auto"
                    ? "autoSignatureNote"
                    : "noSignatures",
                )}
              </p>
            )}
            {current.assessments?.map((a, index) => (
              <div className="assessment" key={`${a.reviewer}-${index}`}>
                <strong>
                  {t(a.model === "human" ? "manual" : "ai")} ·{" "}
                  {t(
                    (
                      {
                        approve: "approveDecision",
                        escalate: "escalateDecision",
                        reject: "rejectDecision",
                        lock: "lockDecision",
                      } as const
                    )[a.decision as "approve" | "escalate" | "reject" | "lock"],
                  )}
                </strong>
                <p>{a.reason}</p>
                <small>
                  {a.model} · {a.reviewer} · {when(a.at)}
                </small>
              </div>
            ))}
            {current.needs_human && current.status === "pending" && (
              <div className="callout warning">{t("humanWarning")}</div>
            )}
            {current.status === "pending" &&
              current.review.mode === "ai" &&
              !current.needs_human &&
              (current.intent.requester === me.id || humanReviewer) && (
                <button
                  className="secondary full"
                  disabled={busy}
                  onClick={() =>
                    void mutate(
                      () =>
                        api(
                          token,
                          `/requests/${current.intent.id}/handoff`,
                          {},
                        ),
                      "handedOff",
                    )
                  }
                >
                  {t("manualHandoff")}
                </button>
              )}
            {current.status === "ready" &&
              current.intent.kind !== "session" &&
              current.intent.requester === me.id && (
                <div className="approval-box">
                  <h4>
                    {t("grantReady")} · {current.intent.max_uses - current.used}{" "}
                    {t("uses")}
                  </h4>
                  <p>{t("sameScope")}</p>
                  <button
                    className="primary full"
                    disabled={busy}
                    onClick={() =>
                      void mutate(
                        () =>
                          api(
                            token,
                            `/requests/${current.intent.id}/execute`,
                            {},
                          ),
                        "queuedAgain",
                      )
                    }
                  >
                    <Terminal size={16} />
                    {t("executeAgain")}
                  </button>
                </div>
              )}
            {current.status === "pending" &&
              humanReviewer &&
              !current.approvals.some((a) => a.subject === me.id) && (
                <div className="approval-box">
                  <h4>
                    <FileKey2 size={17} />
                    {t(demo ? "demoSign" : "independentSign")}
                  </h4>
                  <p>{t("signNote")}</p>
                  {!demo && (
                    <>
                      <pre>
                        safe-devops approve --url https://YOUR-BROKER --request{" "}
                        {current.intent.id} --key /secure/reviewer.key
                      </pre>
                      <label htmlFor="signature">{t("offlineSignature")}</label>
                      <textarea
                        id="signature"
                        value={signature}
                        onChange={(e) => setSignature(e.target.value)}
                        placeholder={t("signaturePlaceholder")}
                      />
                    </>
                  )}
                  <button
                    className="primary full"
                    disabled={busy || (!demo && !signature.trim())}
                    onClick={() =>
                      void mutate(() => approve(current), "signed")
                    }
                  >
                    <Check size={16} />
                    {t(demo ? "approve" : "submitSignature")}
                  </button>
                </div>
              )}
            {current.status === "pending" && humanReviewer && (
              <div className="reject-box">
                <label htmlFor="rejection">{t("rejectReason")}</label>
                <textarea
                  id="rejection"
                  value={rejection}
                  maxLength={1000}
                  onChange={(e) => setRejection(e.target.value)}
                  placeholder={t("rejectPlaceholder")}
                />
                <button
                  className="danger full"
                  disabled={busy || rejection.trim().length < 3}
                  onClick={() =>
                    void mutate(
                      () =>
                        api(token, `/requests/${current.intent.id}/reject`, {
                          reason: rejection,
                        }),
                      "rejectedNotice",
                    )
                  }
                >
                  {t("reject")}
                </button>
              </div>
            )}
            {current.executions?.length > 0 && (
              <div className="execution-history">
                <h4 className="section-label">
                  {t("executionHistory")} · {current.used} /{" "}
                  {current.intent.max_uses}
                </h4>
                {current.executions.map((ex) => (
                  <details key={ex.use}>
                    <summary>
                      {t("run")} {ex.use} · {statusText(ex.status)} ·{" "}
                      {when(ex.started_at)}
                    </summary>
                    <code>{operationLabel(ex.operation)}</code>
                    <pre>{ex.output || t("waitingResult")}</pre>
                  </details>
                ))}
              </div>
            )}
            {current.output !== undefined && (
              <div className="output">
                <h4>
                  <Terminal size={16} />
                  {t("output")}
                  <span>exit {current.exit_code}</span>
                </h4>
                <pre>{current.output || t("noOutput")}</pre>
              </div>
            )}
            {["pending", "approved", "running", "ready"].includes(
              current.status,
            ) &&
              (current.intent.requester === me.id || humanReviewer) && (
                <div className="revoke-box">
                  <button
                    className="danger full"
                    disabled={busy}
                    onClick={() =>
                      void mutate(
                        () =>
                          api(
                            token,
                            `/requests/${current.intent.id}/revoke`,
                            {},
                          ),
                        "revokedNotice",
                      )
                    }
                  >
                    <XCircle size={16} />
                    {t("revoke")}
                  </button>
                  <small>{t("revokeNote")}</small>
                </div>
              )}
          </aside>
        </div>
      )}
    </div>
  );
}

function CreateDialog({
  initialTarget,
  initialAction,
  targets,
  projects,
  busy,
  onClose,
  onCreate,
}: {
  initialTarget?: string;
  initialAction?: string;
  targets: Target[];
  projects: Project[];
  busy: boolean;
  onClose: () => void;
  onCreate: (body: unknown) => Promise<void>;
}) {
  const { t } = usePreferences();
  const initial = targets.find((t) => t.id === initialTarget) || targets[0];
  const initialTask =
    initial?.actions.find((a) => a.id === initialAction) || initial?.actions[0];
  const [project, setProject] = useState(initial?.project_id || "");
  const [target, setTarget] = useState(initial?.id || "");
  const [action, setAction] = useState(initialTask?.id || "");
  const [reason, setReason] = useState("");
  const [ttl, setTTL] = useState(300);
  const [maxUses, setMaxUses] = useState(
    initialTask?.diagnostics ? Math.min(20, initialTask.review.max_uses) : 1,
  );
  const [steps, setSteps] = useState<string[]>([]);
  const actions = targets.find((x) => x.id === target)?.actions || [];
  const selected: Action | undefined = actions.find((a) => a.id === action);
  return (
    <div className="modal-backdrop">
      <section
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-title"
      >
        <div className="drawer-top">
          <span className="eyebrow">{t("newOperation")}</span>
          <button
            className="icon-button"
            aria-label={t("close")}
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </div>
        <h2 id="create-title">{t("createTitle")}</h2>
        <p className="muted">{t("immutable")}</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void onCreate({
              target,
              action,
              reason,
              ttl_seconds: ttl,
              max_uses: maxUses,
              steps: selected?.plan ? steps : undefined,
            });
          }}
        >
          {projects.length > 0 && (
            <>
              <label htmlFor="project">{t("project")}</label>
              <Select
                id="project"
                value={project}
                onValueChange={(value) => {
                  setProject(value);
                  const first = targets.find((x) => x.project_id === value);
                  setTarget(first?.id || "");
                  setAction(first?.actions[0]?.id || "");
                  setMaxUses(1);
                  setSteps([]);
                }}
              >
                {projects
                  .filter((p) => targets.some((x) => x.project_id === p.id))
                  .map((p) => (
                    <SelectItem value={p.id} key={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
              </Select>
            </>
          )}
          <label htmlFor="target">{t("target")}</label>
          <Select
            id="target"
            value={target}
            onValueChange={(value) => {
              setTarget(value);
              setAction(
                targets.find((x) => x.id === value)?.actions[0]?.id || "",
              );
              setMaxUses(1);
              setSteps([]);
            }}
            required
          >
            {targets
              .filter((x) => !project || x.project_id === project)
              .map((x) => (
                <SelectItem value={x.id} key={x.id}>
                  {x.name} / {x.environment}
                </SelectItem>
              ))}
          </Select>
          <label htmlFor="action">{t("task")}</label>
          <Select
            id="action"
            value={action}
            onValueChange={(value) => {
              setAction(value);
              setMaxUses(1);
              setSteps([]);
            }}
            required
          >
            {actions.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </Select>
          {selected && (
            <div className="runbook-hint">
              <ShieldCheck size={19} />
              <div>
                <strong>
                  {t(selected.review.mode)} · {t(selected.review.risk)} ·{" "}
                  {selected.timeout_seconds} {t("seconds")}
                </strong>
                <p>{selected.description}</p>
              </div>
            </div>
          )}
          {selected?.plan && (
            <PlanSelector
              action={selected}
              selected={steps}
              onChange={setSteps}
            />
          )}
          <label htmlFor="reason">{t("reason")}</label>
          <textarea
            id="reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            minLength={reason.length > 0 ? 8 : undefined}
            maxLength={1000}
            required={selected?.review.mode !== "auto"}
            placeholder={t("reasonHint")}
          />
          <div className="form-grid">
            <div>
              <label htmlFor="max-uses">{t("maximumUses")}</label>
              <Select
                id="max-uses"
                value={String(maxUses)}
                onValueChange={(value) => setMaxUses(Number(value))}
              >
                {Array.from(
                  { length: selected?.review.max_uses || 1 },
                  (_, i) => (
                    <SelectItem key={i + 1} value={String(i + 1)}>
                      {i === 0 ? t("once") : `${i + 1} ${t("uses")}`}
                    </SelectItem>
                  ),
                )}
              </Select>
            </div>
            <div>
              <label htmlFor="ttl">{t("expiry")}</label>
              <Select
                id="ttl"
                value={String(ttl)}
                onValueChange={(value) => setTTL(Number(value))}
              >
                {[300, 600, 900].map((seconds) => (
                  <SelectItem value={String(seconds)} key={seconds}>
                    {seconds / 60} {t("minutes")}
                  </SelectItem>
                ))}
              </Select>
            </div>
          </div>
          <div className="form-actions">
            <button type="button" className="secondary" onClick={onClose}>
              {t("cancel")}
            </button>
            <button
              className="primary"
              disabled={
                busy ||
                !selected ||
                (!!selected.plan && steps.length === 0) ||
                (reason.trim().length > 0
                  ? reason.trim().length < 8
                  : selected.review.mode !== "auto")
              }
            >
              {t("submit")}
              <ArrowRight size={16} />
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

function TeamView({
  workspace,
  projects,
  members,
  targets,
}: {
  workspace: Workspace;
  projects: Project[];
  members: Principal[];
  targets: Target[];
}) {
  const { t } = usePreferences();
  return (
    <>
      <div className="panel team-intro">
        <span className="workspace-icon">{workspace.name?.[0] || "S"}</span>
        <div>
          <h2>{workspace.name}</h2>
          <p>
            {workspace.id} · {members.length} {t("members")} · {projects.length}{" "}
            {t("projects")}
          </p>
        </div>
        <span className="pill">WORKSPACE</span>
      </div>
      <div className="asset-grid">
        {projects.map((p) => (
          <section className="panel asset-card" key={p.id}>
            <div className="asset-title">
              <span className="asset-icon">
                <FolderKanban size={22} />
              </span>
              <span className="pill">{p.id}</span>
            </div>
            <h2>{p.name}</h2>
            <p className="muted">{p.description}</p>
            <div className="asset-meta">
              <span>
                {t("assets")}
                <strong>
                  {targets.filter((x) => x.project_id === p.id).length}
                </strong>
              </span>
              <span>
                {t("teamTitle")}
                <strong>{p.members.length}</strong>
              </span>
            </div>
            {targets
              .filter((x) => x.project_id === p.id)
              .map((x) => (
                <div className="agent-row" key={x.id}>
                  <Server size={17} />
                  <div>
                    <strong>{x.name}</strong>
                    <small>
                      {x.environment} · {x.id}
                    </small>
                  </div>
                </div>
              ))}
          </section>
        ))}
      </div>
      <section className="panel team-members">
        <div className="panel-heading">
          <div>
            <h2>
              {t("teamTitle")}
              <span className="count">{members.length}</span>
            </h2>
            <p>{t("managedConfig")}</p>
          </div>
          <Users size={20} />
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>{t("member")}</th>
                <th>{t("identityType")}</th>
                <th>{t("role")}</th>
                <th>{t("projectScope")}</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.id}>
                  <td>
                    <strong>{m.name}</strong>
                    <small className="review-mode muted">{m.id}</small>
                  </td>
                  <td>{t(m.kind === "ai" ? "aiIdentity" : "humanIdentity")}</td>
                  <td>
                    {m.roles
                      .map((r) => t((r + "Role") as TranslationKey))
                      .join(" / ")}
                  </td>
                  <td>
                    {m.roles.includes("auditor")
                      ? t("allTeamProjects")
                      : projects
                          .filter((p) => p.members.includes(m.id))
                          .map((p) => p.name)
                          .join(" / ") || "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}
function PolicyView() {
  const { t } = usePreferences();
  return (
    <>
      <div className="review-policies">
        {[
          {
            mode: "auto",
            icon: Check,
            title: "policyAutoTitle",
            text: "policyAuto",
          },
          {
            mode: "ai",
            icon: Sparkles,
            title: "policyAITitle",
            text: "policyAI",
          },
          {
            mode: "manual",
            icon: FileKey2,
            title: "policyHumanTitle",
            text: "policyHuman",
          },
        ].map(({ mode, icon: Icon, title, text }) => (
          <article className="panel" key={mode}>
            <div className="policy-card-top">
              <span className="asset-icon">
                <Icon size={22} />
              </span>
              <span className={`mode-tag ${mode}`}>
                {t(mode as TranslationKey)}
              </span>
            </div>
            <h3>{t(title as TranslationKey)}</h3>
            <p>{t(text as TranslationKey)}</p>
          </article>
        ))}
      </div>
      <section className="panel boundary-panel">
        <div className="panel-heading">
          <h2>
            <ShieldCheck size={19} />
            {t("boundaryTitle")}
          </h2>
        </div>
        <div className="policy-flow">
          {[
            {
              icon: KeyRound,
              title: "developerBoundary",
              sub: "developerBoundarySub",
            },
            {
              icon: FileKey2,
              title: "reviewerBoundary",
              sub: "reviewerBoundarySub",
            },
            {
              icon: Server,
              title: "connectorBoundary",
              sub: "connectorBoundarySub",
            },
          ].map(({ icon: Icon, title, sub }, i) => (
            <div className="policy-stage" key={title}>
              <span className="stage-number">0{i + 1}</span>
              <Icon size={22} />
              <h3>{t(title as TranslationKey)}</h3>
              <p>{t(sub as TranslationKey)}</p>
            </div>
          ))}
        </div>
        <p className="policy-caution">{t("policyCaution")}</p>
      </section>
      <section className="panel github-panel">
        <GitBranch size={23} />
        <div>
          <h3>{t("githubTitle")}</h3>
          <p>{t("githubNote")}</p>
        </div>
      </section>
    </>
  );
}
