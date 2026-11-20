import { Contact } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.customers", section: "admin.customers", order: 20, label: "Customers", icon: Contact, hue: "var(--color-primary)", cap: "customers.read", feature: "catalog" },
  ],
} satisfies PageMeta;
