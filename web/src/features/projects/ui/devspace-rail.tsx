import { Brain, Code2, FolderTree, Globe, Home, Sparkles, SquareTerminal } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { ToggleButton, ToggleButtonGroup, type Key } from "react-aria-components";
import { Tooltip } from "../../../lib/design/components/tooltip/tooltip";

export type Surface = "home" | "build" | "code" | "terminal" | "previews" | "files" | "rrflow";

/** localStorage key of the last surface per project (the prompt form preselects "build"). */
export const surfaceKey = (p: string) => `shippin.devspace-surface.${p}`;

const items: [Surface, string, LucideIcon, string][] = [
  ["home", "Home", Home, "What's running and how to get back in"],
  ["build", "Build", Sparkles, "Your AI build: progress and its live terminal"],
  ["code", "Code", Code2, "Edit your code"],
  ["terminal", "Terminal", SquareTerminal, "Run commands in this devspace"],
  ["previews", "Previews", Globe, "Open your running app"],
  ["files", "Files", FolderTree, "Browse files"],
  ["rrflow", "rrflow", Brain, "Sessions, memory and the decision log"],
];

/** Left rail of devspace surfaces (RAC ToggleButtonGroup: one selected, arrow-key navigation). */
export function DevspaceRail({ value, onChange }: { value: Surface; onChange: (s: Surface) => void }) {
  return (
    <ToggleButtonGroup
      aria-label="Devspace surfaces"
      orientation="vertical"
      selectionMode="single"
      disallowEmptySelection
      selectedKeys={[value]}
      onSelectionChange={(k: Set<Key>) => k.size && onChange([...k][0] as Surface)}
      className="flex w-20 flex-col items-stretch gap-1 border-r border-line p-2"
    >
      {items.map(([id, label, Icon, hint]) => (
        <Tooltip key={id} content={hint}>
          <ToggleButton
            id={id}
            className="focus-ring flex cursor-default flex-col items-center gap-1 rounded-lg py-2 text-2xs text-fg-muted outline-none data-hovered:bg-raised data-hovered:text-fg data-selected:bg-primary/12 data-selected:text-primary"
          >
            <Icon className="size-5" />
            {label}
          </ToggleButton>
        </Tooltip>
      ))}
    </ToggleButtonGroup>
  );
}
