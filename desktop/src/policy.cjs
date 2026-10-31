"use strict";
function consoleOrigin(value, allowLocal = false) {
  if (typeof value !== "string" || value.length > 2048) throw new Error("Invalid console address / 控制台地址无效");
  const u = new URL(value.trim());
  const local = ["127.0.0.1", "localhost", "[::1]"].includes(u.hostname);
  if (u.username || u.password || u.search || u.hash || (u.pathname !== "/" && u.pathname !== "") || (u.protocol !== "https:" && !(allowLocal && local && u.protocol === "http:"))) {
    throw new Error("Use the HTTPS console origin, without credentials or a path / 请输入 HTTPS 控制台域名，不含凭据和路径");
  }
  return u.origin;
}
function allowedURL(value, origin) {
  try { const u = new URL(value); return !u.username && !u.password && ["https:", "http:", "wss:", "ws:"].includes(u.protocol) && u.host === new URL(origin).host && (u.protocol === "wss:" ? "https:" : u.protocol === "ws:" ? "http:" : u.protocol) === new URL(origin).protocol; } catch { return false; }
}
module.exports = { consoleOrigin, allowedURL };
