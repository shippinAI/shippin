import { defineConfig } from "astro/config";

// The marketing site's canonical host. An operator republishing the site changes this.
export default defineConfig({
  site: "https://shippin.run",
  trailingSlash: "always",
  output: "static",
  compressHTML: true,
  build: { inlineStylesheets: "never" },
});
