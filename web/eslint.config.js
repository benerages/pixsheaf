import svelte from "eslint-plugin-svelte";
import tseslint from "typescript-eslint";
import svelteConfig from "./svelte.config.js";

export default tseslint.config(
  {
    ignores: [
      "node_modules/",
      "../internal/ui/dist/",
      "src/lib/api/schema.d.ts",
      "playwright-report/",
      "test-results/",
    ],
  },
  ...tseslint.configs.strict,
  ...svelte.configs.recommended,
  {
    files: ["**/*.svelte", "**/*.svelte.ts"],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: [".svelte"],
        svelteConfig,
      },
    },
  },
  {
    rules: {
      // Inline styles would require 'unsafe-inline' in the CSP (ADR 0010).
      "svelte/no-inline-styles": "error",
      "svelte/no-at-html-tags": "error",
    },
  },
);
