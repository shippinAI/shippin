# Panel framework

How the panel in `web/` is organised, so that a page, its navigation entry, its components and its words each have one place, and a check in CI says when something drifts. The look itself is the [design system](design-system.md); this page is the structure around it.

The rule behind all of it: **a rule only exists if CI fails when it is broken.** A convention that is only written down decays.

## Routes and navigation come from folders (F1)

No route or navigation entry is listed by hand. Folders under `web/src/routes/` are the URLs:

| File | Meaning |
| --- | --- |
| `page.tsx` | A page at this folder's path. Its default export is the page component. |
| `layout.tsx` | Wraps every page below this folder. Its default export renders an `<Outlet />`. |
| `meta.ts` | Optional. The page's navigation entries, the permission (`cap`) or feature switch it needs, and its place in its section. |
| `[name]/` | A path parameter, `:name`. |
| `(name)/` | A group: shares a layout, adds nothing to the path. |

Each top-level folder under `routes/` is one route tree. The console app mounts `routes/console/`. The provider panel mounts `routes/admin/` under `/admin`, both inside the console and in the operator app.

`npm run routes` writes `web/src/app/routes.gen.ts` from the folders:
- each tree's route objects, every page its own lazily loaded chunk;
- each tree's navigation entries, in section order;
- `RoutePath`, the union of every path, which the link helpers take, so a link to a page that does not exist fails the type check.

The file is checked in so a change to the routes shows in review. `npm test` runs `npm run routes -- --check`, which fails when the file is stale.

A navigation entry keeps its `id`. The installation renames, hides and moves entries by it (`ui.nav.items`), so changing an id breaks a site's configuration.

The tests also fail when:
- two pages claim the same path;
- a navigation entry points at no page;
- a file under `web/src/app` or `web/src/template` lists a route or a navigation entry by hand.

### Modules

A module's pages live in `web/src/modules/<name>/routes/<tree>/`, with the same conventions. The generator reads them into the same trees, so the panel's own build carries none of them, and moving a page between editions means moving its folder. A module page may not claim a path the panel already serves.

### Pages and features

A page folder holds the page and nothing shared. The code a page uses belongs to its feature, `web/src/features/<feature>/`, which owns its API calls, models and parts. A feature imports another feature only through that feature's `index.ts`.

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
