import type { ReactNode } from "react";
import { Tab, TabList, TabPanel, Tabs as RacTabs, type Key } from "react-aria-components";

export type TabDef = { id: string; label: string; content: ReactNode };

type Props = {
  tabs: TabDef[];
  label: string;
  defaultTab?: string;
  /** Controlled selection, for tabs that live in the URL. */
  selected?: string;
  onSelect?: (id: string) => void;
};

/** Underlined tab bar (RAC) like cloud-console resource pages. */
export function Tabs({ tabs, label, defaultTab, selected, onSelect }: Props) {
  const control = selected !== undefined ? { selectedKey: selected, onSelectionChange: (k: Key) => onSelect?.(String(k)) } : {};
  return (
    <RacTabs defaultSelectedKey={tabs.some((t) => t.id === defaultTab) ? defaultTab : undefined} {...control}>
      <TabList aria-label={label} className="mb-6 flex gap-6 border-b border-line">
        {tabs.map((t) => (
          <Tab
            key={t.id}
            id={t.id}
            className="focus-ring relative -mb-px cursor-default border-b-2 border-transparent pb-3 text-sm text-fg-muted outline-none transition-colors data-hovered:text-fg data-selected:border-primary data-selected:text-fg"
          >
            {t.label}
          </Tab>
        ))}
      </TabList>
      {tabs.map((t) => (
        <TabPanel key={t.id} id={t.id} className="outline-none">
          {t.content}
        </TabPanel>
      ))}
    </RacTabs>
  );
}
