import { createBrowserRouter } from "../lib/router/router";
import { toRoutes } from "../lib/router/tree";
import { routes } from "./routes.console.gen";

// The console's routes come from the folders under src/routes/console (docs/web-framework.md). Every page
// is its own chunk; only the root layout and the sign-in gate load with the app (initial JS budget).
export const router = createBrowserRouter(toRoutes(routes));
