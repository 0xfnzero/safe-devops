export type DiagnosticOperation = {
  tool: string;
  resource?: string;
  path?: string;
  query?: string;
  lines?: number;
};
export type DiagnosticPolicy = {
  roots: { id: string; path: string }[];
  tools: string[];
  auto_tools?: string[];
};
export type ChangeStep = {
  id: string;
  name: string;
  description: string;
  argv: string[];
  timeout_seconds: number;
};
export type PlanPolicy = {
  steps: ChangeStep[];
  max_steps: number;
  privileged: boolean;
};
export type RiskContext = {
  version: number;
  captured_at: number;
  since: number;
  truncated: boolean;
  records: {
    request_id: string;
    action: string;
    operation?: string;
    status: string;
    at: number;
    exit_code?: number;
    output?: string;
  }[];
};
export function operationLabel(o?: DiagnosticOperation) {
  return o
    ? [
        o.tool,
        o.resource ? `${o.resource}:${o.path}` : "",
        o.query ? JSON.stringify(o.query) : "",
        o.lines ? String(o.lines) : "",
      ]
        .filter(Boolean)
        .join(" ")
    : "";
}
export type Principal = {
  id: string;
  name: string;
  roles: string[];
  public_key?: string;
  kind?: "human" | "ai";
};
export type DemoUser = {
  id: string;
  name: string;
  role: string;
  token: string;
  seed?: string;
};
export type ReviewPolicy = {
  mode: "auto" | "ai" | "manual";
  risk: "low" | "medium" | "high";
  max_uses: number;
  rate_limit_per_minute: number;
};
export type Action = {
  diagnostics?: DiagnosticPolicy;
  plan?: PlanPolicy;
  review: ReviewPolicy;
  id: string;
  name: string;
  description: string;
  quorum: number;
  command_hash: string;
  timeout_seconds: number;
  approvers: string[];
};
export type Workspace = { id: string; name: string };
export type Project = {
  id: string;
  name: string;
  description?: string;
  members: string[];
};
export type Target = {
  project_id?: string;
  id: string;
  name: string;
  environment: string;
  requesters: string[];
  actions: Action[];
};
export type Intent = {
  kind?: "session" | "command" | "plan";
  scope?: DiagnosticPolicy;
  operation?: DiagnosticOperation;
  plan?: ChangeStep[];
  workspace_id?: string;
  project_id?: string;
  max_uses: number;
  id: string;
  target: string;
  action: string;
  requester: string;
  reason: string;
  created_at: number;
  expires_at: number;
  command_hash: string;
};
export type Request = {
  operation?: DiagnosticOperation;
  risk_context?: RiskContext;
  intent: Intent;
  payload: string;
  fingerprint: string;
  status: string;
  approvals: { subject: string; at: number; kind?: string }[];
  review: ReviewPolicy;
  used: number;
  needs_human: boolean;
  assessments: {
    context_digest?: string;
    reviewer: string;
    decision: string;
    reason: string;
    model: string;
    at: number;
  }[];
  executions: {
    operation?: DiagnosticOperation;
    use: number;
    status: string;
    started_at: number;
    output?: string;
    exit_code?: number;
  }[];
  quorum: number;
  output?: string;
  exit_code?: number;
  lease_expires?: number;
};
export type Audit = {
  sequence: number;
  at: number;
  actor: string;
  event: string;
  request_id: string;
  detail: string;
  hash: string;
  previous: string;
};
export async function api<T>(
  token: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  const data = await response.json().catch(() => ({ error: "服务暂时不可用" }));
  if (!response.ok)
    throw new Error(data.error || `请求失败 (${response.status})`);
  return data;
}
// Deliberately only used with keys generated for loopback demo mode.
export async function demoSignature(
  seed: string,
  payload: string,
  domain = "safeops.approval.v1\n",
): Promise<string> {
  const prefix = Uint8Array.from([
    0x30, 0x2e, 0x02, 0x01, 0, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04,
    0x22, 0x04, 0x20,
  ]);
  const keyBytes = new Uint8Array(48);
  keyBytes.set(prefix);
  keyBytes.set(
    Uint8Array.from(atob(seed), (c) => c.charCodeAt(0)),
    16,
  );
  const key = await crypto.subtle.importKey(
    "pkcs8",
    keyBytes,
    { name: "Ed25519" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign(
    "Ed25519",
    key,
    new TextEncoder().encode(domain + payload),
  );
  return btoa(String.fromCharCode(...new Uint8Array(signature)));
}
export type AccountLock = {
  id: string;
  workspace_id: string;
  requester: string;
  project_id: string;
  target: string;
  action: string;
  source_id: string;
  reviewer: string;
  reason: string;
  model: string;
  context_digest: string;
  created_at: number;
  expires_at: number;
  released_at?: number;
  released_by?: string;
};
export type LockItem = {
  lock: AccountLock;
  payload: string;
  fingerprint: string;
  can_unlock: boolean;
};
export type RiskScan = {
  id: string;
  requester: string;
  target: string;
  project_id: string;
  created_at: number;
  expires_at: number;
  risk_context: RiskContext;
  assessment?: Request["assessments"][number];
};
