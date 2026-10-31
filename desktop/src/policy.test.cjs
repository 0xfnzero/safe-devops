const { test } = require("node:test"); const assert = require("node:assert/strict");
const { consoleOrigin, allowedURL } = require("./policy.cjs");
test("console address requires trusted HTTPS; local HTTP is development only", () => {
  assert.equal(consoleOrigin("https://ops.example.com/"), "https://ops.example.com");
  for (const u of ["http://ops.example.com", "http://127.0.0.1:3000", "file:///etc/passwd", "javascript:alert(1)", "https://a:b@ops.example.com", "https://ops.example.com/path", "https://ops.example.com/?token=secret", "https://ops.example.com/#fragment"]) assert.throws(() => consoleOrigin(u));
  assert.equal(consoleOrigin("http://127.0.0.1:3000", true), "http://127.0.0.1:3000");
  assert.throws(() => consoleOrigin("http://localhost.attacker.test", true));
});
test("remote rendering cannot navigate or connect outside configured origin", () => {
  const origin = "https://ops.example.com";
  for (const u of [origin + "/api/requests", "wss://ops.example.com/socket"]) assert.equal(allowedURL(u, origin), true);
  for (const u of ["http://ops.example.com", "https://ops.example.com.attacker.test", "file:///etc/passwd", "data:text/html,test", "https://ops.example.com:444", "https://attacker.test", "https://user:pass@ops.example.com", "ws://ops.example.com/socket"]) assert.equal(allowedURL(u, origin), false);
});
