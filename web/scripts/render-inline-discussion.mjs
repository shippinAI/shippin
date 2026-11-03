import { readFile, writeFile } from "node:fs/promises";
import { createJiti } from "jiti";

const [input, output, ...allowedOrigins] = process.argv.slice(2);
if (!input || !output || !allowedOrigins.length) {
  throw new Error("Usage: node scripts/render-inline-discussion.mjs snapshot.json output.html https://trusted-origin");
}
const jiti = createJiti(import.meta.url);
const { parseDiscussion } = await jiti.import("../src/features/discussions/discussion.ts");
const discussion = parseDiscussion(JSON.parse(await readFile(input, "utf8")), allowedOrigins);
const template = await readFile(new URL("../src/features/discussions/inline-fragment.html", import.meta.url), "utf8");
// A message must never terminate the JSON script element or become executable markup.
const json = JSON.stringify(discussion).replaceAll("<", "\\u003c").replaceAll(">", "\\u003e").replaceAll("&", "\\u0026");
const fragment = template.replace("__DISCUSSION_DATA__", () => json);
if (Buffer.byteLength(fragment, "utf8") > 1_000_000) throw new Error("Discussion exceeds the inline size limit");
await writeFile(output, fragment, "utf8");
console.log(`Rendered ${discussion.messages.length} messages to ${output}`);
