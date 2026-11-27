import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

export default {
  preprocess: vitePreprocess(),
  compilerOptions: {
    // Component CSS goes into external files, never injected at runtime
    // (strict CSP without 'unsafe-inline'; ADR 0010).
    css: "external",
  },
};
