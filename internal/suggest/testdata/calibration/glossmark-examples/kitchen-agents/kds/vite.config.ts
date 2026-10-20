import { defineConfig } from "vite";

export default defineConfig({
  server: { port: 5174, proxy: { "/api": { target: "http://localhost:8080", rewrite: (p) => p.replace(/^\/api/, "") } } },
  test: { environment: "jsdom", globals: true },
});
