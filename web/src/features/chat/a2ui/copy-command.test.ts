import assert from "node:assert/strict";
import { test } from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CopyCommand } from "./copy-command";

test("copy command renders the exact vendor text", () => {
  const command = "gpg --show-keys key.asc | grep -q '31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE'";
  const html = renderToStaticMarkup(createElement(CopyCommand, { command }));
  assert.ok(html.includes("31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE"));
  assert.equal(html.includes("ANTHROPIC_API_KEY"), false);
});
