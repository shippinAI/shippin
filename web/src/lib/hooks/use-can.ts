import { useAdminAccess } from "./use-admin";

/** The granular customer-catalog permissions (deny by default: Owner or a direct grant). */
export const CatalogPerm = {
  plansRead: "catalog.plans.read",
  pricesRead: "catalog.prices.read",
  plansWrite: "catalog.plans.write",
  defaultsWrite: "config.defaults.write",
  customersRead: "customers.read",
  contactsRead: "customers.contacts.read",
  notesRead: "customers.notes.read",
  customersWrite: "customers.write",
  servicesWrite: "customers.services.write",
  effectiveRead: "config.effective.read",
  intentCreate: "provision.intent.create",
} as const;

/** can(p) from the caller's effective permissions (/v1/operator/access). The API enforces the
 * same decision (403, or the field is left out of the response); hiding here is only UX. */
export function useCan() {
  const perms = useAdminAccess().data?.staff?.permissions ?? [];
  return (p: string) => perms.includes(p);
}
