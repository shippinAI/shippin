import { createContext, useContext } from "react";
import type { A2UIUserAction, Surface } from "../../lib/a2ui/a2ui-types";
import { A2UINode } from "./a2ui-node";

type Ctx = {
  surface: Surface;
  onAction: (a: A2UIUserAction) => void;
  onData: (path: string, value: unknown) => void;
};

const SurfaceCtx = createContext<Ctx | null>(null);

export function useSurface(): Ctx {
  const c = useContext(SurfaceCtx);
  if (!c) throw new Error("A2UI node outside a surface");
  return c;
}

type Props = Ctx & { label?: string; className?: string };

/** Renders one A2UI v0.9 surface from its "root" component with React Aria controls. Until root
 * arrives nothing shows (spec: buffer until root is defined). */
export function A2UISurface({ surface, onAction, onData, label, className }: Props) {
  if (!surface.components.root) return null;
  return (
    <SurfaceCtx.Provider value={{ surface, onAction, onData }}>
      <div className={className} data-a2ui-surface={surface.id} aria-label={label} role={label ? "group" : undefined}>
        <A2UINode id="root" scope="" />
      </div>
    </SurfaceCtx.Provider>
  );
}
