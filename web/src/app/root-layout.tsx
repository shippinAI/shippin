import { lazy, Suspense, useEffect } from "react";
import { RouterProvider } from "react-aria-components";
import { Outlet, useHref, useNavigate } from "../lib/router/router";
import { usePublicSettings } from "../lib/hooks/use-api";

const ToastRegionHost = lazy(() => import("../lib/design/components/toast/toast-region").then((m) => ({ default: m.ToastRegionHost })));

/** Bridges RAC links to the router, sets the title, hosts toasts. */
export function RootLayout() {
  const navigate = useNavigate();
  const settings = usePublicSettings().data;
  const brand = settings?.brand.name;
  const color = settings?.brand.color;
  useEffect(() => {
    if (brand) document.title = `${brand} Console`;
  }, [brand]);
  useEffect(() => {
    if (color && /^#[0-9a-fA-F]{6}$/.test(color)) {
      document.documentElement.style.setProperty("--color-primary", color);
      document.documentElement.style.setProperty("--color-primary-strong", color);
    }
  }, [color]);
  return (
    <RouterProvider navigate={navigate} useHref={useHref}>
      <Outlet />
      <Suspense fallback={null}>
        <ToastRegionHost />
      </Suspense>
    </RouterProvider>
  );
}
