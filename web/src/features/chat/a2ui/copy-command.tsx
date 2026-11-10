import { createElement } from "react";

/** The exact command, on screen, so a paste still works if the clipboard is blocked. */
export function CopyCommand({ command }: { command: string }) {
  return createElement(
    "pre",
    { className: "max-h-40 w-full overflow-auto whitespace-pre-wrap rounded-sm border border-line bg-canvas p-2 font-mono text-xs leading-snug text-fg" },
    command,
  );
}

export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    /* the command stays on screen to copy by hand */
  }
}
