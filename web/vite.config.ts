import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// The build output is embedded into the Go binary (internal/ui).
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: {
    outDir: "../internal/ui/dist",
    emptyOutDir: true,
    // The polyfill would be injected as an inline script, which the CSP
    // forbids; all supported browsers implement modulepreload natively.
    modulePreload: { polyfill: false },
    assetsInlineLimit: 0,
    sourcemap: false,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/healthz": "http://127.0.0.1:8080",
    },
  },
});
