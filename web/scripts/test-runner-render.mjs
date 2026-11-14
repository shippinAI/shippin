import { createJiti } from "jiti";
const jiti = createJiti(import.meta.url, { jsx: { runtime: "automatic" } });
await jiti.import("../src/features/runners/render.test.ts");
