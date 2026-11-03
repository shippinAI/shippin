import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The dev server forwards API calls to a running backend and presents its origin, because the
// console refuses writes from any other. No special mode exists in the server for development.
const api = process.env.SHIPPIN_DEV_API ?? "https://localhost:8443";
const backend = { target: api, secure: false, changeOrigin: true, headers: { origin: api } };

// Two apps, one codebase (CONSOLE-IA): the customer console (index.html) and the operator app
// (ops.html, served on the operator origin), each with its own entry bundle.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { proxy: { "/v1": backend, "/auth": backend } },
  build: { target: "es2022", sourcemap: false, rollupOptions: { input: { index: "index.html", ops: "ops.html", launcher: "launcher.html" } } },
});
