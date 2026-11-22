import { createBrowserRouter } from "../lib/router/router";
import { toRoutes } from "../lib/router/tree";
import { routes } from "./routes.ops.gen";

/** The operator app (ops.<domain>): only the provider panel, its own bundle and sign-in (src/routes/ops). */
export const opsRouter = createBrowserRouter(toRoutes(routes));
