# ADR 0010: Frontend stack

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

The UI needs a strict CSP without `unsafe-inline`, has one or two screens, and is embedded with `go:embed`. SvelteKit with adapter-static emits an inline bootstrap script that needs hashes, and delivers its CSP through a `<meta>` tag where `frame-ancestors` is ignored ([SvelteKit config](https://svelte.dev/docs/kit/configuration)).

## Options

(a) A plain Svelte 5 + Vite 8 SPA. (b) SvelteKit 3 with adapter-static.

## Decision

Option (a), with Tailwind 4.3.5 through `@tailwindcss/vite`, component CSS emitted to files (never `css: 'injected'`), and the `svelte/no-inline-styles` lint rule. Use Paraglide JS 3.x for English and German instead of svelte-i18n, which has been unmaintained since 2024. Pin TypeScript `~6.0.x`, because typescript-eslint 8.74.0 requires `<6.1.0` and svelte-check 4.7.8 requires `^5 || ^6` ([typescript-eslint](https://registry.npmjs.org/typescript-eslint/latest), [svelte-check](https://registry.npmjs.org/svelte-check/latest)). Use pnpm 12.23.0 through `packageManager`, and Node 26 LTS at build time only.

## Consequences

The Go server can send `script-src 'self'; style-src 'self'` as headers. Svelte 5 transitions use the Web Animations API and need no inline styles ([svelte#6662](https://github.com/sveltejs/svelte/issues/6662)). Revisit TypeScript 7 when 7.1 is stable and the tools update their peer ranges. Not yet verified: that Paraglide uses no `eval`, and Vite 8's modulepreload polyfill behavior.
