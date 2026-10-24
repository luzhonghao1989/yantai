import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./style.css";
import { configureAPI } from "./types";
import ServerGate from "./ServerGate";
async function start() {
  const w = window as unknown as {
    go?: {
      main?: {
        App?: { APIInfo?: () => Promise<{ base: string; token: string }> };
      };
    };
  };
  if (w.go?.main?.App?.APIInfo) configureAPI(await w.go.main.App.APIInfo());
  createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <ServerGate><App /></ServerGate>
    </React.StrictMode>,
  );
}
start().catch((e) => {
  document.getElementById("root")!.textContent = "工作台启动失败：" + e.message;
});
