import { lazy, Suspense, type ComponentType } from "react";
import type { RouteObject } from "./match";

type Loaded = { default: ComponentType };

/** One route as scripts/routes.mjs writes it from the route folders. */
export type GenRoute = {
  path?: string;
  /** A page, loaded on demand. */
  page?: () => Promise<Loaded>;
  /** A layout loaded on demand, or one loaded with the app. */
  load?: () => Promise<Loaded>;
  layout?: ComponentType;
  children?: GenRoute[];
};

/** The router's routes for generated ones. Call it once per app: each call makes its own lazy components. */
export function toRoutes(gen: GenRoute[]): RouteObject[] {
  return gen.map((g) => {
    if (g.page) {
      const Page = lazy(g.page);
      return { path: g.path, element: <Suspense fallback={null}><Page /></Suspense> };
    }
    const children = toRoutes(g.children ?? []);
    if (g.layout) {
      const Layout = g.layout;
      return { path: g.path, element: <Layout />, children };
    }
    if (g.load) {
      const Layout = lazy(g.load);
      return { path: g.path, element: <Suspense fallback={null}><Layout /></Suspense>, children };
    }
    return { path: g.path, children };
  });
}
