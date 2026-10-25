import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./fonts.css";
import "./tokens.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <App session={document.body.dataset.session ?? "local"} />
  </StrictMode>,
);
