import assert from "node:assert/strict";
import { actionPath } from "./actions";

// Path parameters fill the route; a declared query parameter goes in the query string, once.
assert.equal(actionPath("shippin.tenancy.customer.get", { customerId: "a b" }), "/v1/operator/tenancy/customers/a%20b");
assert.equal(actionPath("shippin.tenancy.customer.effective_config.get", { customerId: "c1", workspace: "w 1" }),
  "/v1/operator/tenancy/customers/c1/effective-config?workspace=w+1");
assert.equal(actionPath("shippin.tenancy.customer.effective_config.get", { customerId: "c1", workspace: undefined }),
  "/v1/operator/tenancy/customers/c1/effective-config");
assert.throws(() => actionPath("shippin.tenancy.customer.get", {}), /needs customerId/);
