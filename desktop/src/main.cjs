"use strict";
const { app, BrowserWindow, ipcMain, dialog, safeStorage, net } = require("electron");
const crypto = require("node:crypto");
const device = require("./device.cjs");
let deviceKey;
let deviceLoading;
const fs = require("node:fs/promises");
const path = require("node:path");
const { pathToFileURL } = require("node:url");
const { consoleOrigin, allowedURL } = require("./policy.cjs");
let launcher;
const consoles = new Set();
const launcherURL = pathToFileURL(path.join(__dirname, "launcher.html")).href;
const allowLocal = !app.isPackaged && process.env.SAFE_DEVOPS_ALLOW_LOCAL === "1";
async function loadDevice() {
  if (deviceKey) return deviceKey;
  if (!deviceLoading) deviceLoading = initializeDevice().finally(() => { deviceLoading = undefined; });
  return deviceLoading;
}
async function initializeDevice() {
  if (!safeStorage.isEncryptionAvailable() || (process.platform === "linux" && safeStorage.getSelectedStorageBackend() === "basic_text")) throw new Error("OS key protection is unavailable / 操作系统密钥保护不可用，请启用系统密钥环");
  const file = path.join(app.getPath("userData"), "device-key.enc");
  let pem;
  try { pem = safeStorage.decryptString(await fs.readFile(file)); }
  catch (e) { if (e.code !== "ENOENT") throw new Error("Cannot decrypt enrolled device key; refusing to replace it / 无法解密设备密钥，请恢复系统密钥环");
    pem = crypto.generateKeyPairSync("ed25519").privateKey.export({format:"pem",type:"pkcs8"});
    await fs.writeFile(file, safeStorage.encryptString(pem), {mode:0o600,flag:"wx"});
  }
  deviceKey=crypto.createPrivateKey(pem); return deviceKey;
}
async function verifyMachine(origin, key) {
  const status = await net.fetch(`${origin}/api/device/status`, {redirect:"error",signal:AbortSignal.timeout(10000)});
  if (!status.ok) throw new Error("Console must be upgraded to support device verification / 控制台需升级以支持设备验证");
  const policy = await status.json(); if (!policy.required) return;
  const headers=device.signRequest(key,"POST","/api/device/verify",Buffer.alloc(0),"");
  const response=await net.fetch(`${origin}/api/device/verify`,{method:"POST",headers,redirect:"error",signal:AbortSignal.timeout(10000)});
  if(!response.ok) throw new Error("Machine not enrolled or IP not allowed. Give the device public key to your administrator / 设备未登记或 IP 不允许，请将设备公钥交给管理员授权");
}
function lockPermissions(session) {
  session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
  session.setPermissionCheckHandler(() => false);
  session.on("will-download", event => event.preventDefault());
}
function isLauncher(event) { return launcher && !launcher.isDestroyed() && event.sender === launcher.webContents && event.senderFrame?.url === launcherURL; }
function makeLauncher() {
  launcher = new BrowserWindow({ width: 680, height: 770, minWidth: 480, minHeight: 650, title: "safe-devops", backgroundColor: "#131316", webPreferences: { preload: path.join(__dirname, "preload.cjs"), sandbox: true, contextIsolation: true, nodeIntegration: false, webviewTag: false, partition: "launcher" } });
  launcher.removeMenu(); lockPermissions(launcher.webContents.session);
  launcher.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  launcher.webContents.on("will-navigate", e => e.preventDefault());
  void launcher.loadFile(path.join(__dirname, "launcher.html"));
}
async function openConsole(origin) {
  const key=await loadDevice(); await verifyMachine(origin,key);
  const win = new BrowserWindow({ width: 1440, height: 960, minWidth: 800, minHeight: 600, title: "safe-devops", backgroundColor: "#131316", webPreferences: { sandbox: true, contextIsolation: true, nodeIntegration: false, webSecurity: true, webviewTag: false, safeDialogs: true, navigateOnDragDrop: false, partition: `console-${Date.now()}-${Math.random()}` } });
  consoles.add(win); win.removeMenu();
  const session = win.webContents.session;
  lockPermissions(session);
  session.webRequest.onBeforeRequest((details, callback) => callback({ cancel: !allowedURL(details.url, origin) }));
  session.webRequest.onBeforeSendHeaders(async (details, callback) => {
    try {
      if (!allowedURL(details.url,origin)) return callback({cancel:true});
      const url=new URL(details.url); if(!url.pathname.startsWith("/api/")) return callback({requestHeaders:details.requestHeaders});
      const headers={...details.requestHeaders};
      const auth=Object.entries(headers).find(([name])=>name.toLowerCase()==="authorization")?.[1] || "";
      for(const name of Object.keys(headers)) if(name.toLowerCase().startsWith("x-safe-devops-")) delete headers[name];
      const body=await device.requestBytes(details.uploadData,session);
      Object.assign(headers,device.signRequest(key,details.method,url.pathname+url.search,body,auth.startsWith("Bearer ")?auth.slice(7):""));
      callback({requestHeaders:headers});
    } catch { callback({cancel:true}); }
  });
  win.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  for (const name of ["will-navigate", "will-redirect"]) win.webContents.on(name, (event, url) => { if (!allowedURL(url, origin)) event.preventDefault(); });
  win.webContents.on("will-attach-webview", event => event.preventDefault());
  win.on("closed", () => { consoles.delete(win); void session.clearStorageData(); if (!consoles.size && !launcher?.isDestroyed()) launcher.show(); });
  try { await win.loadURL(`${origin}/?view=terminal`); if (!launcher?.isDestroyed()) launcher.hide(); }
  catch { win.close(); throw new Error("Console unavailable or certificate untrusted / 控制台不可用或证书不受信任"); }
}
app.whenReady().then(() => {
  ipcMain.handle("settings", async event => {
    if (!isLauncher(event)) throw new Error("Denied");
    let origin = ""; try { origin = consoleOrigin(JSON.parse(await fs.readFile(path.join(app.getPath("userData"), "console.json"), "utf8")).origin, allowLocal); } catch {}
    const key=await loadDevice();return { origin, allowLocal, device:device.identity(key) };
  });
  ipcMain.handle("connect", async (event, value) => {
    if (!isLauncher(event)) throw new Error("Denied");
    try { const origin = consoleOrigin(value, allowLocal); await openConsole(origin); await fs.writeFile(path.join(app.getPath("userData"), "console.json"), JSON.stringify({ origin }), { mode: 0o600 }); return { ok: true }; }
    catch (e) { return { ok: false, error: e.message }; }
  });
  makeLauncher();
});
app.on("window-all-closed", () => app.quit());
app.on("activate", () => { if (!BrowserWindow.getAllWindows().length) makeLauncher(); });
app.on("certificate-error", (event, _contents, _url, _error, _certificate, callback) => { event.preventDefault(); callback(false); });
process.on("uncaughtException", () => { dialog.showErrorBox("safe-devops", "The client encountered an error. Restart the application. / 客户端发生错误，请重启应用。"); app.exit(1); });
