"use client";
import { useState } from "react";
import { LockKeyhole, ShieldAlert, UnlockKeyhole } from "lucide-react";
import {
  api,
  demoSignature,
  type DemoUser,
  type LockItem,
  type RiskScan,
} from "@/lib/api";
import { usePreferences } from "@/lib/preferences";
export function LockBanner({
  items,
  requester,
  now,
}: {
  items: LockItem[];
  requester: string;
  now: number;
}) {
  const { locale, when } = usePreferences();
  const en = locale === "en";
  return items
    .filter(
      ({ lock: l }) =>
        l.requester === requester && !l.released_at && l.expires_at > now,
    )
    .map(({ lock: l }) => (
      <div
        key={l.id}
        className="message error account-lock-banner"
        role="alert"
      >
        <LockKeyhole size={20} />
        <div>
          <strong>
            {en
              ? "Account temporarily locked by risk review"
              : "账户已被风险审核临时锁定"}
          </strong>
          <p>{l.reason}</p>
          <small>
            {en
              ? "New operations blocked. Active grants revoked. Expires: "
              : "新操作已阻断，活动授权已撤销。锁定截止："}
            {when(l.expires_at)}
            {en
              ? ". Contact an independent human reviewer to release it early."
              : "。可联系独立人工审核者提前解锁。"}
          </small>
        </div>
      </div>
    ));
}
export function RiskCenter({
  items,
  scans,
  token,
  demoUser,
  now,
  refresh,
}: {
  items: LockItem[];
  scans: RiskScan[];
  token: string;
  demoUser?: DemoUser;
  now: number;
  refresh: () => Promise<void>;
}) {
  const { locale, when } = usePreferences();
  const en = locale === "en";
  const [signature, setSignature] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <div className="risk-center">
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              <ShieldAlert size={18} />
              {en ? "Account containment" : "账户风险控制"}
            </h2>
            <p>
              {en
                ? "AI may temporarily lock requesters. Early release requires an independent human signature. Revoked grants remain revoked."
                : "AI 可临时锁定申请账户，提前解锁需要独立人工签名。解锁不会恢复已撤销的授权。"}
            </p>
          </div>
        </div>
        {error && (
          <div className="message error" role="alert">
            {error}
          </div>
        )}
        {items.length === 0 && (
          <div className="empty">
            {en ? "No account locks" : "暂无账户锁定记录"}
          </div>
        )}
        {[...items].reverse().map((item) => {
          const l = item.lock;
          const active = !l.released_at && l.expires_at > now;
          return (
            <div className="risk-lock-card" key={l.id}>
              <div className="risk-lock-heading">
                <LockKeyhole size={18} />
                <strong>{l.requester}</strong>
                <span className={`badge ${active ? "failed" : "completed"}`}>
                  {active
                    ? en
                      ? "Locked"
                      : "已锁定"
                    : l.released_at
                      ? en
                        ? "Released by human"
                        : "人工已解锁"
                      : en
                        ? "Expired"
                        : "已到期"}
                </span>
              </div>
              <p>{l.reason}</p>
              <small>
                {l.reviewer} · {l.model} · {when(l.created_at)} →{" "}
                {when(l.expires_at)}
              </small>
              <code className="context-digest">
                {en ? "Evidence digest" : "证据摘要"}: {l.context_digest}
              </code>
              {active && item.can_unlock && (
                <div className="risk-unlock">
                  <p>
                    {en
                      ? "Investigate the workstation and rotate compromised credentials before releasing access."
                      : "请先检查工作站并更换泄露凭据，再恢复账户访问。"}
                  </p>
                  {!demoUser?.seed && (
                    <>
                      <pre>
                        safe-devops unlock-account --url https://YOUR-BROKER
                        --lock {l.id} --key /secure/reviewer.key
                      </pre>
                      <label htmlFor={`unlock-${l.id}`}>
                        {en
                          ? "Signature from independent device"
                          : "独立设备生成的解锁签名"}
                      </label>
                      <textarea
                        id={`unlock-${l.id}`}
                        value={signature[l.id] || ""}
                        onChange={(e) =>
                          setSignature({ ...signature, [l.id]: e.target.value })
                        }
                      />
                      <code className="context-digest">
                        {en ? "Lock fingerprint" : "锁定指纹"}:{" "}
                        {item.fingerprint}
                      </code>
                    </>
                  )}
                  <button
                    className="secondary"
                    disabled={
                      busy || (!demoUser?.seed && !signature[l.id]?.trim())
                    }
                    onClick={async () => {
                      setBusy(true);
                      setError("");
                      try {
                        const sig = demoUser?.seed
                          ? await demoSignature(
                              demoUser.seed,
                              item.payload,
                              "safeops.account-unlock.v1\n",
                            )
                          : signature[l.id].trim();
                        await api(token, `/account-locks/${l.id}/unlock`, {
                          signature: sig,
                        });
                        await refresh();
                      } catch (e) {
                        setError((e as Error).message);
                      } finally {
                        setBusy(false);
                      }
                    }}
                  >
                    <UnlockKeyhole size={15} />
                    {en ? "Sign and unlock" : "签名并解锁"}
                  </button>
                </div>
              )}
            </div>
          );
        })}
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{en ? "Recent behavior assessments" : "近期行为评估"}</h2>
            <p>
              {en
                ? "Completed operations, including automatic commands, trigger retrospective AI checks. Outputs are scoped, redacted and clipped."
                : "已执行的操作（包括自动放行命令）会触发 AI 行为巡检，输出按范围收集、脱敏和截断。"}
            </p>
          </div>
        </div>
        {scans.length === 0 && (
          <div className="empty">
            {en ? "No assessments yet" : "暂无行为评估"}
          </div>
        )}
        {scans.map((scan) => (
          <details className="risk-scan" key={scan.id}>
            <summary>
              {scan.requester} · {scan.target} ·{" "}
              {scan.assessment?.decision === "lock"
                ? en
                  ? "Account locked"
                  : "已触发锁定"
                : scan.assessment?.decision === "approve"
                  ? en
                    ? "No containment"
                    : "无需锁定"
                  : scan.assessment
                    ? en
                      ? "Human investigation needed"
                      : "需要人工调查"
                    : now >= scan.expires_at
                      ? en
                        ? "Review timed out"
                        : "巡检超时"
                      : en
                        ? "Assessing…"
                        : "评估中…"}{" "}
              · {when(scan.created_at)}
            </summary>
            <p>{scan.assessment?.reason}</p>
            {scan.risk_context.records.map((r, n) => (
              <div key={n}>
                <strong>
                  {r.operation || r.action} · {r.status}
                  {r.exit_code !== undefined && ` · exit ${r.exit_code}`}
                </strong>
                <pre>{r.output}</pre>
              </div>
            ))}
          </details>
        ))}
      </section>
    </div>
  );
}
