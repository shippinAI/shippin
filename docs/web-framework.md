# Panel framework

How the panel in `web/` is organised, so that a page, its navigation entry, its components and its words each have one place, and a check in CI says when something drifts. The look itself is the [design system](design-system.md); this page is the structure around it.

The rule behind all of it: **a rule only exists if CI fails when it is broken.** A convention that is only written down decays.

Status checked 2026-11-20: F1 routes/navigation and F5 checks are implemented.
F4's product lexicon and F2's token-generation source are planned; the documented
`docs/lexicon.toml` and `web/tokens/` do not yet exist. Existing CSS token enforcement
remains implemented. F3 has components, while its complete gallery acceptance
still needs verification. The [foundation delivery plan](foundation-delivery.md)
defines context grouping, shell loading states and incremental migration using
this machinery. The sections below specify the target where tooling is absent.

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

`npm test` is `scripts/test.mjs`. It runs the checks, then every test, and reports each step that failed. The web job in CI runs it.

- **Every test runs.** Test files are found by name, not listed by hand, so a new test cannot be left out:
  - `src/**/*.test.ts` run one by one with jiti;
  - `scripts/**/*.test.mjs` run with `node --test`.
- **Boundaries** (`scripts/check-boundaries.mjs`):
  - `web/src/lib` imports no feature, page, template, app or module code. Its tests may.
  - `web/src/features/` gains no folder. A feature is a context's panel code, and new work goes inside the feature of its context. `scripts/features-baseline.txt` lists today's folders and only shrinks.
  - A feature reaches another only through that feature's `index.ts`. The 52 imports into other features' internals that existed when the check arrived are listed in `scripts/boundaries-baseline.txt`. That list may only shrink: a new import fails, and so does a listed one that is gone until its line is removed. The check also compares the file with its version at the merge base (main, or the pull request's base) and fails if a line was added, so writing the new import into the baseline does not make it pass.
  - Nothing outside `web/src/modules` imports a module. The generated route files only name module folders.
  - No folder is a bucket (`common`, `shared`, `utils`, `util`, `helpers`, `misc`). Code belongs to the thing it serves.
- **Generated files are current.** The routes (`scripts/routes.mjs --check`) and the OpenAPI index (`generate-openapi-index.mjs --check`) are regenerated in check mode, and a difference fails. The tokens join them with F2.
- **Hard-coded installation names** fail too (`scripts/check-hardcoding.mjs`), as before.
- **Formatting.** Biome formats the web code (`web/biome.json`: two-space indent, 140 columns, double quotes, trailing commas). `npm test` runs `biome format` and fails on a file that is not formatted; `npx biome format --write` fixes it. Generated files are left as their generators write them. The commit that first formatted everything is in `.git-blame-ignore-revs`.
- **Lint.** Biome's recommended rules run over the web code (`scripts/check-lint.mjs`). The 176 errors and warnings that existed when it arrived are counted per file and rule in `scripts/lint-baseline.txt`.
- **Nothing unused.** knip (`web/knip.json`, `scripts/check-unused.mjs`) finds files, exports, types and dependencies nothing uses. Route folders are entry points. The 121 findings that existed when it arrived are in `scripts/unused-baseline.txt`.
- **How the two baselines work.** Both may only shrink (`scripts/baseline.mjs`). A finding above its count fails, and so does a count above what is found, so a fix is recorded with `--update`, which lowers counts and never adds one.

## Lexicon (F4, planned)

`docs/lexicon.toml` defines each product term once:
- what it means;
- the words that must not be used for it, such as *tenant* for organization in anything a person reads.

It holds product terms only, never a person's own vocabulary.

- **Generated words for the UI.** `npm run lexicon` writes `web/src/lib/lexicon/terms.gen.ts`, so the UI takes a term's label from there instead of typing it.
- **What the check scans and fails on.** The check reads the panel's UI text and the docs, and fails on a banned word.
- **The ratchet.** Uses that exist today are listed in `web/scripts/lexicon-baseline.txt`. That list may only shrink, the same way the MCPL list of older routes does.
- **Retired terms.** A retired term is merged into the term that replaced it. It is not kept as an alias.

## Tokens from one source (F2, generation planned)

The tokens in the [design system](design-system.md) are written once, in the DTCG format, under `web/tokens/`. Style Dictionary builds them into:
- `web/src/styles/tokens.css`, for both the dark and the light theme;
- the Tailwind theme;
- TypeScript types.

`npm test` builds them in check mode. `npm run tokens` keeps enforcing how they are used.

## Components (F3)

- **The folder.** Each component in `web/src/lib/design/components/<name>/` has `<name>.tsx`, `index.ts` and, when it has logic, `<name>.test.ts`. Button is the reference example.
- **The gallery.** **Settings > Interface > Components** shows every component in every state. A test fails when a component is missing from it.
- **When one is added.** A component is added when a screen needs it, not ahead of time.
