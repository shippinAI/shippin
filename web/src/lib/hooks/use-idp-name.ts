import { usePublicSettings } from "./use-api";

/** The identity product name users see (config idp.name, e.g. "Nomen"); never the vendor name. */
export const useIdpName = () => usePublicSettings().data?.idp_name || "your identity provider";
