import { test } from "node:test";
import assert from "node:assert/strict";
import { deviceSignInURL } from "./device-sign-in";

test("Nomen sign-in carries the exact approval destination without tab storage", () => {
  const login = new URL(deviceSignInURL("ABCD-EFGH"), "https://console.example.test");
  assert.equal(login.pathname, "/auth/login");
  assert.equal(login.searchParams.get("rd"), "/device?code=ABCD-EFGH");
});

test("an edited code cannot inject another return destination", () => {
  const code = "x&rd=//other.example/#fragment";
  const login = new URL(deviceSignInURL(code), "https://console.example.test");
  const back = new URL(login.searchParams.get("rd")!, login.origin);
  assert.equal(back.origin, login.origin);
  assert.equal(back.pathname, "/device");
  assert.equal(back.searchParams.get("code"), code);
  assert.equal(back.searchParams.has("rd"), false);
});
