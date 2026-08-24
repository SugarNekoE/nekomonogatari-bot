---
name: bootstrap-3
description: Build, revise, or review Bootstrap 3.4.1 user interfaces in Nori's SSR-and-hydrated SvelteKit web app. Use when a task asks for Bootstrap 3 markup, grids, forms, navigation, panels, alerts, modals, dropdowns, tooltips, popovers, Glyphicons, responsive utilities, jQuery plugins, Svelte client interactions, or browser-only UI libraries that must fit this repository's Bootstrap setup. Also use to diagnose Bootstrap 3 component structure or JavaScript behavior and prevent accidental Bootstrap 4/5 syntax.
---

# Bootstrap 3

Create Bootstrap 3.4.1 interfaces that match this app's SSR-and-hydrated SvelteKit architecture and existing vendored assets.

## Establish context

1. Read [references/repo-and-docs.md](references/repo-and-docs.md).
2. Inspect the target route, its nearest layout, `src/app.html`, and nearby routes before editing.
3. Consult the linked official Bootstrap 3.4 documentation for the exact component being used. Do not reconstruct fragile component markup from memory.
4. Preserve the existing asset strategy unless the task explicitly changes it.

## Implement the interface

- Use Bootstrap 3 classes and documented DOM structures. Do not use Bootstrap 4/5 classes, utilities, custom properties, or `data-bs-*` attributes.
- Prefer Bootstrap's existing grid, components, and utilities over custom CSS. Add narrowly scoped CSS only when Bootstrap 3 cannot express the requirement.
- Keep `.row` inside `.container` or `.container-fluid`, and place only `.col-xs-*`, `.col-sm-*`, `.col-md-*`, or `.col-lg-*` elements directly inside a row.
- Design mobile-first. Add the smallest breakpoint class that expresses the intended behavior, then add larger breakpoint overrides only as needed.
- Preserve semantic HTML and keyboard behavior. Use `<button type="button">` for actions and `<a href="...">` for navigation.
- Include the complete documented markup for components such as navbars, modals, dropdowns, form groups, input groups, and dismissible alerts. Bootstrap 3 selectors depend on structure.
- Give interactive controls unique IDs and connect labels and ARIA attributes to those IDs.
- Make every `aria-describedby` value resolve to an element that is present in the same response. Use `role="alert"` or another live region for newly reported errors, not for persistent instructional help.
- In horizontal forms, let `.form-group` provide the row behavior; do not also add `.row` to the same element.
- Use Glyphicons only through Bootstrap 3's `.glyphicon` classes, with accessible text when the icon conveys meaning.

## Fit SvelteKit SSR and hydration

- Keep SSR enabled. The root layout enables CSR so server-rendered HTML hydrates and subsequent navigation can use the SvelteKit client router.
- Fetch initial page data in server `load` functions and perform authoritative mutations through form actions. Use client state for interaction, not as a second source of truth.
- Use Bootstrap data attributes such as `data-toggle`, `data-target`, and `data-dismiss` for simple supported jQuery plugins. Use Svelte handlers and state when a workflow has application state, validation, streaming, or progress updates.
- Do not let jQuery and Svelte independently own the same stateful DOM behavior. Define one lifecycle owner for each widget and clean up plugin or library instances when its component is destroyed.
- Keep internal navigation as real links. When a path must respect SvelteKit's configured base path, use `resolve` from `$app/paths`.
- Do not add another Bootstrap or jQuery import to a route. Global assets already load from `static/vendor` in `src/app.html`, with jQuery before Bootstrap.
- Initialize opt-in tooltips and popovers after their elements mount. The initializer in `src/app.html` covers the initial document only; client-rendered elements need a Svelte action or component lifecycle integration, with teardown.
- Load browser-only libraries such as CodeMirror and xterm.js inside `onMount` or another browser-only boundary, preferably with dynamic imports. Destroy editors, terminals, subscriptions, observers, and timers during teardown.
- Use Ajv in the browser for immediate JSON Schema feedback, but repeat authoritative validation on the server.
- Provide an SSR-rendered placeholder or read-only fallback for client-only editors and log viewers so loading and failure states remain understandable.

## Validate

1. Compare the final DOM structure and attributes with the relevant Bootstrap 3.4 docs.
2. Check narrow and wide viewport behavior, including grid wrapping and navbar collapse.
3. Exercise every interactive widget: open and close it, use the keyboard, and verify focus and ARIA state where applicable.
4. Run `pnpm --filter @nori/web check` from the monorepo root, or `pnpm check` from this app.
5. Run the narrowest relevant tests. Use Playwright for browser behavior when practical.
6. Review the diff for Bootstrap 4/5 leakage, duplicate asset loading, missing `type="button"`, duplicate IDs, broken internal links, and avoidable custom CSS.

## Escalate architectural changes

Treat upgrades, dependency changes, replacing vendored assets, disabling SSR or hydration, or mixing another component system into Bootstrap pages as architectural work. Explain the tradeoff and obtain direction when the request does not already authorize it. Bootstrap 3 is end-of-life, but this skill's job is to work safely within the repository's explicit version choice rather than silently upgrade it.
