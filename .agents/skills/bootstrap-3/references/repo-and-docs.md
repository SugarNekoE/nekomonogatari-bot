# Repository baseline and Bootstrap 3 documentation

## Repository baseline

- `src/app.html` loads `/vendor/css/bootstrap.min.css`.
- `src/app.html` loads `/vendor/js/jquery-1.12.4.min.js` before `/vendor/js/bootstrap.min.js`.
- `src/app.html` globally initializes tooltips and popovers.
- `static/vendor/` contains Bootstrap 3.4.1 CSS, JavaScript, Glyphicons, and jQuery. Reuse these files; do not add CDN tags or package dependencies for ordinary UI work.
- `src/routes/+layout.ts` exports `csr = true` and `prerender = false`. Pages are server-rendered, then hydrated, and subsequent navigation can use the SvelteKit client router.
- `src/routes/+layout.svelte` demonstrates the repository's navbar structure and root `.container`.
- `src/routes/+page.svelte` demonstrates panels, alerts, dropdowns, tooltips, and a modal driven by Bootstrap data attributes.
- The project uses Svelte 5 runes mode and Prettier formatting.
- Initial data belongs in server `load` functions and authoritative mutations belong in form actions. CodeMirror and xterm.js load behind browser-only lifecycle boundaries; Ajv browser validation is repeated authoritatively on the server.

Reinspect these files before relying on this summary because repository state can change.

## Official Bootstrap 3.4.1 docs

Use the narrowest relevant page and section:

- [Getting started](https://getbootstrap.com/docs/3.4/getting-started/) — assets, jQuery requirement, starter document, browser support, and accessibility.
- [CSS](https://getbootstrap.com/docs/3.4/css/) — containers, the 12-column grid, typography, tables, forms, buttons, images, helpers, and responsive utilities.
- [Components](https://getbootstrap.com/docs/3.4/components/) — Glyphicons, dropdowns, button groups, input groups, navigation, navbars, breadcrumbs, pagination, labels, badges, page headers, thumbnails, alerts, progress bars, media objects, list groups, panels, embeds, and wells.
- [JavaScript](https://getbootstrap.com/docs/3.4/javascript/) — plugin dependencies, data APIs, events, transitions, modals, dropdowns, scrollspy, tabs, tooltips, popovers, alerts, buttons, collapse, carousel, and affix.
- [Accessibility guidance](https://getbootstrap.com/docs/3.4/getting-started/#accessibility) — semantic markup, ARIA, color contrast, and keyboard limitations.

Bootstrap 3.4.1 is end-of-life. Use these versioned pages as the API contract and do not substitute current Bootstrap documentation.

## Version traps

Reject or translate patterns from newer Bootstrap versions:

- Use `data-toggle`, `data-target`, and `data-dismiss`, never `data-bs-toggle`, `data-bs-target`, or `data-bs-dismiss`.
- Use `.pull-left`, `.pull-right`, `.center-block`, and `.hidden-*`/`.visible-*` when their documented Bootstrap 3 behavior is appropriate; newer spacing, flexbox, display, and directional utility classes do not exist.
- Use `.panel`, `.well`, `.thumbnail`, `.navbar-default`, and `.glyphicon` where appropriate. `.card`, `.badge-*`, `.navbar-expand-*`, and Bootstrap Icons are not Bootstrap 3 APIs.
- Use `.col-xs-*`, `.col-sm-*`, `.col-md-*`, and `.col-lg-*`. There are no `.col-*`, `.col-xl-*`, or CSS-grid gutters in Bootstrap 3.
- Use `.form-group`, `.control-label`, `.form-control`, `.help-block`, and Bootstrap 3 validation state structure. Do not assume newer form-control markup.
- Bootstrap 3 JavaScript plugins depend on jQuery and documented DOM structure. Do not rewrite them as Bootstrap 5 native-JavaScript calls.

## Interactive behavior map

- Data attributes are normally sufficient for collapse, dropdowns, modals, dismissible alerts, tabs, and carousels.
- Tooltips and popovers require explicit initialization. `src/app.html` initializes elements in the initial document; elements created by client navigation or conditional rendering need lifecycle-aware initialization and teardown.
- Programmatic control and custom event handling must use the Bootstrap 3 jQuery plugin API and documented event names.
- Plugins must not be initialized twice. Check `src/app.html`, Svelte actions, and component lifecycle code before adding initialization.
- Bootstrap jQuery plugins and Svelte must not independently control the same widget state. Choose one lifecycle owner and destroy stateful plugin instances before their DOM is removed.
