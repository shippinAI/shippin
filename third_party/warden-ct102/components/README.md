# warden/components/ — UI Component Partials

Per ADR 0038 + Spec 17. Each file defines a single Go `html/template`
named partial that pages compose into a full page.

`components.shell` renders a page-specific `.Shell.BodyTemplate` such as
`customers.body` or `workspaces.body`. Do not create a shared global
`shell_body` block for new pages; every page body name must be unique in the
global Go template set.

## Authoring rule

```html
{{ define "components.<name>" }}
   <component html — class-based, no inline styles />
{{ end }}
```

The Go-side template registration in `warden/main.go` adds this
directory's `*.html` to the template root:

```go
tmpl, err := template.New("").Funcs(funcs).ParseFS(
    templatesFS,
    "templates/*.html",
    "components/*.html",
)
```

## Catalog (Spec 17 §3)

| File | Component | Purpose |
|---|---|---|
| `shell.html` | `components.shell` | Page wrapper — full `<!DOCTYPE html>` doc |
| `breadcrumb.html` | `components.breadcrumb` | Drill-down trail |
| `section.html` | `components.section` | Titled content area |
| `toolbar.html` | `components.toolbar` | Action group |
| `entity_card.html` | `components.entity_card` | Drill-into card primitive |
| `entity_list.html` | `components.entity_list` | List/grid of entity_cards |
| `service_row.html` | `components.service_row` | ADR 0037 customer-service row |
| `status_pill.html` | `components.status_pill` | Colored state pill |
| `metric_spark.html` | `components.metric_spark` | Inline metric + sparkline |
| `action_button.html` | `components.action_button` | Primary/secondary/destructive button |
| `confirm_dialog.html` | `components.confirm_dialog` | Native `<dialog>` confirmation |
| `empty_state.html` | `components.empty_state` | Empty-list placeholder |
| `workspace_card.html` | `components.workspace_card` | Independent workspace boundary |
| `connection_row.html` | `components.connection_row` | Explicit registry edge |

## Styling rule

- All classes prefixed `c-` (component): `.c-status-pill`,
  `.c-entity-card`, etc.
- Color uses the coldlight OKLCH CSS custom properties from
  `warden/static/wardenui.css` per Spec 17 section 2, with hex fallbacks
  for older renderers.
- Sizing uses `rem` plus `clamp()`, `min()`, and `max()` tokens from
  `wardenui.css`; do not introduce fixed pixel-only component geometry.
- NO inline `style=""` attributes in new components or pages.
- Variants use BEM-style modifiers: `.c-status-pill--active`,
  `.c-action-button--destructive`.

## Anti-patterns

- ❌ Net-new partials without a Spec 17 catalog entry — open a PR
  updating Spec 17 first.
- ❌ Inline styles inside components.
- ❌ Per-component JavaScript files. Use a single
  `warden/static/wardenui.js` for shared behavior; otherwise lean on
  HTMX 2.x.
- ❌ React / Vue / Svelte. ADR 0002 + ADR 0038 §3 hold.
