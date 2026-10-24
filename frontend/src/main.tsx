import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/fonts.css";
import "./styles/tokens.css";
import "./styles/base.css";

async function start() {
  let initialView;
  if (import.meta.env.MODE === "preview") {
    initialView = (await import("./preview/init")).initPreview();
  }
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App initialView={initialView} />
    </StrictMode>,
  );
}
void start();
