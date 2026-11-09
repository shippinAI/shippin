import type { Guide } from "../../../lib/guides/guide-types";
import { adoptFolderGuide } from "./adopt-folder";
import { cloneRepoGuide } from "./clone-repo";
import { connectAccountsGuide } from "./connect-accounts";
import { publishProjectGuide } from "./publish-project";

/** The guides the Terminal view offers, in menu order. */
export const terminalGuides: Guide[] = [connectAccountsGuide, cloneRepoGuide, adoptFolderGuide, publishProjectGuide];
