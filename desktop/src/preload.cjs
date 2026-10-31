"use strict";
const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("safeDevops", Object.freeze({ settings: () => ipcRenderer.invoke("settings"), connect: origin => ipcRenderer.invoke("connect", origin) }));
