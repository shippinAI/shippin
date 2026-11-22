import { defineConfig } from "astro/config";

const site = process.env.SITE_URL;
if (site) {
  const url = new URL(site);
  if (url.protocol !== "https:" || url.username || url.password || url.search || url.hash || url.pathname !== "/") {
    throw new Error("SITE_URL must be an HTTPS origin without credentials, path, query or fragment");
  }
}

// A local preview has no published canonical host. Deployments supply their own.
export default defineConfig({
  site,
  trailingSlash: "always",
  output: "static",
  compressHTML: true,
  build: { inlineStylesheets: "never" },
});
