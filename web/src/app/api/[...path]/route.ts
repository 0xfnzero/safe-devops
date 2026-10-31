import type { NextRequest } from "next/server";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

// Runtime configuration allows one signed web artifact to be installed in
// different environments. Client input never selects an upstream origin.
async function proxy(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> },
) {
  const { path } = await context.params;
  if (path.some((p) => !/^[a-zA-Z0-9_-]+$/.test(p)) || path[0] === "agent")
    return Response.json(
      { error: "API path not available through Web" },
      { status: 404 },
    );
  const configured = process.env.SAFEOPS_API_ORIGIN;
  if (!configured && process.env.NODE_ENV === "production")
    return Response.json(
      { error: "SAFEOPS_API_ORIGIN is required" },
      { status: 503 },
    );
  let origin: URL;
  try {
    origin = new URL(configured || "http://127.0.0.1:8088");
    if (
      origin.username ||
      origin.password ||
      origin.search ||
      origin.hash ||
      origin.pathname !== "/" ||
      !["https:", "http:"].includes(origin.protocol)
    )
      throw new Error();
    if (
      origin.protocol !== "https:" &&
      (process.env.NODE_ENV === "production" ||
        !["localhost", "127.0.0.1", "[::1]"].includes(origin.hostname))
    )
      throw new Error();
  } catch {
    return Response.json(
      { error: "Invalid Broker origin configuration" },
      { status: 503 },
    );
  }
  try {
    const chunks: Uint8Array[] = [];
    let length = 0;
    if (request.method === "POST" && request.body) {
      const reader = request.body.getReader();
      while (true) {
        const part = await reader.read();
        if (part.done) break;
        length += part.value.length;
        if (length > 32768) {
          await reader.cancel();
          return Response.json({ error: "Request too large" }, { status: 413 });
        }
        chunks.push(part.value);
      }
    }
    const headers = new Headers({ "Content-Type": "application/json" });
    const auth = request.headers.get("authorization");
    if (auth) headers.set("Authorization", auth);
    for (const key of [
      "X-Safe-Devops-Device",
      "X-Safe-Devops-Time",
      "X-Safe-Devops-Nonce",
      "X-Safe-Devops-Signature",
    ]) {
      const value = request.headers.get(key);
      if (value && value.length <= 512) headers.set(key, value);
    }
    // Opt in only behind a private ingress that overwrites client-supplied XFF.
    // The Broker must independently trust this proxy's exact address/CIDR.
    if (process.env.SAFEOPS_TRUST_PROXY === "true") {
      const chain = request.headers.get("x-forwarded-for");
      if (chain && chain.length <= 512) headers.set("X-Forwarded-For", chain);
    }
    if (request.nextUrl.search)
      return Response.json(
        { error: "API query strings are not supported" },
        { status: 400 },
      );
    const response = await fetch(new URL(`/api/${path.join("/")}`, origin), {
      method: request.method,
      headers,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(12000)]),
      body: request.method === "POST" ? Buffer.concat(chunks) : undefined,
    });
    if (response.status >= 300 && response.status < 400) throw new Error();
    return new Response(response.body, {
      status: response.status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    });
  } catch {
    return Response.json({ error: "控制端暂时不可用" }, { status: 503 });
  }
}
export const GET = proxy;
export const POST = proxy;
