const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('sovereignDesktop', {
  getState: () => ipcRenderer.invoke('desktop:get-state'),
  getPlatformInfo: () => ipcRenderer.invoke('desktop:get-platform-info'),
  connectPayload: (payload) => ipcRenderer.invoke('desktop:connect-payload', payload),
  disconnect: () => ipcRenderer.invoke('desktop:disconnect'),
  openWebsite: () => ipcRenderer.invoke('desktop:open-website'),
  onHandoff: (callback) => ipcRenderer.on('desktop:handoff', (_event, payload) => callback(payload)),
  onStateChanged: (callback) => ipcRenderer.on('desktop:state-changed', (_event, state) => callback(state)),
});
