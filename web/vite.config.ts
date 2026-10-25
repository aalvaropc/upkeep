import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vitest/config";

const api = "http://127.0.0.1:8080";

const keepEmbedFolder = (): Plugin => ({
  name: "keep-embed-folder",
  apply: "build",
  closeBundle() {
    writeFileSync(fileURLToPath(new URL("../internal/web/dist/.keep", import.meta.url)), "");
  },
});

export default defineConfig({
  plugins: [react(), keepEmbedFolder()],
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: {
      "/api": api,
      "/healthz": api,
    },
  },
  test: {
    environment: "jsdom",
  },
});
