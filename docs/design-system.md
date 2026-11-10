# Design system

The panel's look is **Flat Vector 2.0**: flat surfaces, hairline borders, one
accent, colour that means something, and dense, legible data. This page is the
rule set for every screen in `web/` (UI-02, UI-03, PERF-01 in
[What Shippin is](objective.md)). A token check in `web/` (`npm run tokens`),
run by CI, enforces it once the token rewrite lands; until then about 500 raw
values in the panel break these rules.

## Tokens are the only source of style

Every colour, font size, weight, line height, letter spacing, radius, shadow and
breakpoint comes from [`web/src/styles/tokens.css`](../web/src/styles/tokens.css).
Tailwind's own default theme is switched off in those namespaces, so a class
such as `text-red-500`, `shadow-lg` or `backdrop-blur` does not exist and draws
nothing. Spacing and sizes use the 4 px grid (`p-4` is 16 px).

| Group | Tokens | Use |
| --- | --- | --- |
| Surfaces | `canvas`, `surface`, `raised`, `overlay`, `scrim` | Page, cards, controls on cards, menus and popovers, modal backdrop. |
| Lines | `line`, `line-strong` | Borders and dividers. |
| Text | `fg`, `fg-muted`, `fg-subtle` | Primary text, secondary text, hints and metadata. |
| Meaning | `primary`, `success`, `warning`, `danger`, `info`, `focus` | Actions, health and state. Never decoration. |
| Modules | `hue-workspaces`, `hue-secrets`, `hue-identity`, `hue-network`, `hue-ai`, `hue-billing`, `hue-data`, `hue-edge` | Telling modules and chart series apart. |
| Type | `text-2xs` 11, `text-xs` 12, `text-sm` 13, `text-base` 14, `text-lg` 16, `text-xl` 20, `text-2xl` 24, `text-3xl` 30 | Body text is `text-base`. Tables and dense lists use `text-sm`. |
| Weight | `font-normal`, `font-medium`, `font-semibold` | Headings are semibold, never bold. |
| Radius | `rounded-sm` 4, `rounded-md` 6, `rounded-lg` 8, `rounded-xl` 12, `rounded-2xl` 16 | Controls `lg`, cards `xl`, dialogs `2xl`. |
| Elevation | `shadow-e1`, `shadow-e2`, `shadow-e3` | Cards, raised panels, overlays. |
| Layout | `max-w-page` and the `max-w-xs` to `max-w-7xl` widths | Page and column widths. |

The shipped theme is dark. A light theme will reuse these names, so no screen
changes when it arrives.

## What the check refuses

- A class that only works with Tailwind's default theme, such as a palette colour, a default shadow or blur.
- Arbitrary values in brackets, such as `text-[13px]` or `bg-[#0b0d12]`. Grid and row templates (`grid-cols-[1fr_auto]`), transition property lists, viewport-relative sizes and React Aria's `--trigger-width` are allowed, because they describe structure, not style.
- Colour literals (`#…`, `rgb(…)`, `oklch(…)`) anywhere in `web/src` except the token file.
- Inline `style` with a literal colour or pixel value. Inline style is only for data-driven numbers such as a meter's width or a chart's coordinates.
- Decorative effects: glass, blur, glow, gradients and background patterns.

## Components

Screens are built from the components in
[`web/src/lib/design/components`](../web/src/lib/design/components), which wrap
[React Aria](https://react-spectrum.adobe.com/react-aria/) primitives. React Aria
supplies keyboard handling, focus management and screen-reader semantics; the
components supply the look from tokens. No other component kit, template library
or copied component collection is added. A feature that needs a new visual
pattern adds a component here, not one-off classes in the feature.

## Accessibility (UI-03)

The bar is WCAG 2.2 AA.

- Text contrast is at least 4.5:1, large text and interface parts at least 3:1, in both themes.
- Everything works from the keyboard, in a sensible order, with a visible `focus` ring.
- Click and touch targets are at least 24 by 24 px.
- Every control has a visible label or an accessible name. Icons that act are buttons with names.
- Colour is never the only signal: health and state also have text or an icon.
- Background updates announce themselves through a polite live region; errors say what happened and what to do.
- Motion is short and functional, and stops under `prefers-reduced-motion`.
- Each chart has a text summary and its numbers available as a table.

## Speed (PERF-01)

- Every route is its own chunk. The first load stays under 90 KB of compressed JavaScript and each route under 50 KB; `npm run budget` enforces it.
- Moving towards a link (hover or focus) starts loading its code and data, so the page is ready on click.
- Data already in the cache draws at once and refreshes in the background; nothing blanks while it refreshes.
- Live metrics arrive as a stream, not by re-fetching whole pages.
- Charts use a small built-in canvas and SVG component, not a charting library.
- Space for content that loads later is reserved, so nothing jumps.
