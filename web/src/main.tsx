import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Root } from "./Root";
import "./fonts.css";
import "./tokens.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
