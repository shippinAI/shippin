import { createContext, useContext, useLayoutEffect, useMemo, useSyncExternalStore, type ReactNode } from "react";
import { navigate, snapshot, subscribe, type Loc } from "./location";
import { matchRoutes, type Match, type RouteObject } from "./match";

export type Router = { routes: RouteObject[] };
type Ctx = { matches: Match[]; depth: number };
const MatchCtx = createContext<Ctx>({ matches: [], depth: 0 });

/** Same call shape as react-router's createBrowserRouter (data APIs are not used). */
export const createBrowserRouter = (routes: RouteObject[]): Router => ({ routes });

export const useLocation = (): Loc => useSyncExternalStore(subscribe, snapshot);

/** Renders the matched branch; nested elements render their child through <Outlet />. */
export function RouterProvider({ router }: { router: Router }) {
  const { pathname } = useLocation();
  const matches = useMemo(() => matchRoutes(router.routes, pathname) ?? [], [router, pathname]);
  if (matches.length === 0) return <NotFound />;
  return <Level matches={matches} depth={0} />;
}

function Level({ matches, depth }: Ctx) {
  const m = matches[depth];
  if (!m) return null;
  const el: ReactNode = m.route.element ?? <Outlet />;
  return <MatchCtx.Provider value={{ matches, depth }}>{el}</MatchCtx.Provider>;
}

export function Outlet() {
  const { matches, depth } = useContext(MatchCtx);
  return <Level matches={matches} depth={depth + 1} />;
}

export function useParams(): Record<string, string | undefined> {
  const { matches } = useContext(MatchCtx);
  return useMemo(() => Object.assign({}, ...matches.map((m) => m.params)), [matches]);
}

export const useNavigate = () => navigate;
export const useHref = (to: string) => to;
export const useSearchParams = (): [URLSearchParams] => {
  const { search } = useLocation();
  return [useMemo(() => new URLSearchParams(search), [search])];
};

/** Redirect while rendering (replace by default is off, like react-router). */
export function Navigate({ to, replace }: { to: string; replace?: boolean }) {
  useLayoutEffect(() => navigate(to, { replace }), [to, replace]);
  return null;
}

function NotFound() {
  return <div className="flex h-full items-center justify-center text-base text-fg-muted">Page not found.</div>;
}
