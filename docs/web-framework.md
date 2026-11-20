# Panel framework

How the panel in `web/` is organised, so that a page, its navigation entry, its components and its words each have one place, and a check in CI says when something drifts. The look itself is the [design system](design-system.md); this page is the structure around it.

The rule behind all of it: **a rule only exists if CI fails when it is broken.** A convention that is only written down decays.

## Routes and navigation come from folders (F1)

No route or navigation entry is listed by hand. Each folder under `web/src/routes/<app>/` is a URL of that app:

| File | Meaning |
| --- | --- |
| `page.tsx` | A page at this folder's path. Its default export is the page component. |
| `layout.tsx` | Wraps every page below this folder. Its default export renders an `<Outlet />`. |
| `meta.ts` | Optional. The navigation entries declared here (`nav`), and `eager: true` on a layout that loads with the app instead of on demand. |
| `mounts.ts` | Optional. `export default ["<app>"]` places another app's routes here. The folder's path is the mounted app's own. |
| `[name]/` | A path parameter, `:name`. |
| `[...name]/` | The rest of the path, `*`. |
| `(name)/` | A group: shares a layout, adds nothing to the path. |

There are three apps:

| App | Folder | Served |
| --- | --- | --- |
| The console | `routes/console/` | The panel itself |
| The provider panel | `routes/admin/`, whose pages are under `admin/` so their paths start with `/admin` | Under `/admin` in the console, loaded on demand |
| The operator app | `routes/ops/` | `ops.<domain>`. It mounts the provider panel with `routes/ops/(signed-in)/admin/mounts.ts` |

### What the generator writes

`npm run routes` writes two files per app into `web/src/app/`:
- `routes.<app>.gen.ts`: the app's routes. Every page loads on demand, and only layouts marked `eager` load with the app. It also exports `RoutePath`, the union of the app's page paths, for typed links.
- `nav.<app>.gen.ts`: every `meta.ts` of the app and the page it sits beside. It is a separate file so the code that starts the app carries no navigation entries. The sidebar's lists (`template/nav-items.ts`, `template/admin/admin-nav.ts`) read it.

Both files are checked in, so a change to the routes shows in review.

### How routes are matched

At every level, a fixed segment is tried before a parameter, and a parameter before the rest of the path. A page beside a group is tried before anything inside the group.

A `page.tsx` that only re-exports a feature's page (`export { XPage as default } from "…"`) is loaded from that feature directly, so it costs no extra chunk.

### Navigation entries

- An entry links to its page's own path.
- Entries under `/t/:tenantId` link relative to the organization.
- `to` is only for an entry that links to one value of a parameter, such as `go/terminal` on `go/[feature]`.
- An entry marked `soon` may sit in a folder with no page.
- An entry keeps its `id`. The installation renames, hides and moves entries by it (`ui.nav.items`), so changing an id breaks a site's configuration.

### What `npm test` checks

- `node scripts/routes.mjs --check` fails when a generated file is stale.
- `scripts/routes.test.mjs` tests the generator's rules: folder names, ordering, modules, mounts, and two folders claiming one path.
- `src/app/routes.test.ts` fails when:
  - a navigation entry points at no page, sits in a section the sidebar does not show, or repeats an id;
  - a file under `web/src/app` or `web/src/template` lists a route, loads a page itself, or lists a navigation entry.

Links still take plain strings. Moving them onto `RoutePath` is the next step for this part.

### Modules

A module's pages live in `web/src/modules/<name>/routes/<app>/`, with the same conventions. The generator reads them into the same apps, so the panel's own build carries none of them, and moving a page between editions means moving its folder. A module may not add a page or a layout where the panel already has one.

### Pages and features

A page folder holds the page and nothing shared. The code a page uses belongs to its feature, `web/src/features/<feature>/`, which owns its API calls, models and parts. Today most `page.tsx` files re-export their feature's page; a page moves into its folder when its feature is next worked on.

## Rot checks (F5)

All of these run in `npm test` or the web job in CI:

- **Every test runs.** Test files are found by name (`*.test.ts`, `*.test.mjs`), not listed by hand, so a new test cannot be left out.
- **Nothing unused.** knip fails on files, exports and dependencies nothing uses.
- **Boundaries.**
  - A feature reaches another only through its `index.ts`.
  - `web/src/lib` imports no feature.
  - Nothing outside `web/src/modules` imports a module.
- **No bucket folders.** No folder is named `common`, `shared`, `utils`, `helpers` or `misc`. Code belongs to the thing it serves.
- **Generated files are current.** Routes, the typed API calls, the OpenAPI index and the tokens are regenerated in check mode, and a difference fails.

## Lexicon (F4)

`docs/lexicon.toml` defines each product term once:
- what it means;
- the words that must not be used for it, such as *tenant* for organization in anything a person reads.

It holds product terms only, never a person's own vocabulary.

- **Generated words for the UI.** `npm run lexicon` writes `web/src/lib/lexicon/terms.gen.ts`, so the UI takes a term's label from there instead of typing it.
- **What the check scans and fails on.** The check reads the panel's UI text and the docs, and fails on a banned word.
- **The ratchet.** Uses that exist today are listed in `web/scripts/lexicon-baseline.txt`. That list may only shrink, the same way the MCPL list of older routes does.
- **Retired terms.** A retired term is merged into the term that replaced it. It is not kept as an alias.

## Tokens from one source (F2)

The tokens in the [design system](design-system.md) are written once, in the DTCG format, under `web/tokens/`. Style Dictionary builds them into:
- `web/src/styles/tokens.css`, for both the dark and the light theme;
- the Tailwind theme;
- TypeScript types.

`npm test` builds them in check mode. `npm run tokens` keeps enforcing how they are used.

## Components (F3)

- **The folder.** Each component in `web/src/lib/design/components/<name>/` has `<name>.tsx`, `index.ts` and, when it has logic, `<name>.test.ts`. Button is the reference example.
- **The gallery.** **Settings > Interface > Components** shows every component in every state. A test fails when a component is missing from it.
- **When one is added.** A component is added when a screen needs it, not ahead of time.
