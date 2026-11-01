import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Two apps, one codebase (CONSOLE-IA): the customer console (index.html) and the operator app
// (ops.html, served on the operator origin), each with its own entry bundle.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { proxy: { "/v1": "http://localhost:8080", "/auth": "http://localhost:8080" } },
  build: { target: "es2022", sourcemap: false, rollupOptions: { input: { index: "index.html", ops: "ops.html", launcher: "launcher.html" } } },
});
