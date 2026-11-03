import { useState } from "react";
import { Form } from "react-aria-components";
import { Button } from "../../../lib/design/components/button/button";
import { Select } from "../../../lib/design/components/select/select";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { A2UIBlock } from "../../../lib/chat/chat-types";

/** An A2UI form: on submit, {field} placeholders in its prompt are filled and the prompt is sent. */
export function A2UIForm({ b, onPrompt }: { b: A2UIBlock; onPrompt: (text: string) => void }) {
  const [values, setValues] = useState<Record<string, string>>({});
  const set = (k: string, v: string) => setValues((x) => ({ ...x, [k]: v }));
  const submit = () => {
    let text = b.prompt ?? "";
    for (const f of b.fields ?? []) text = text.split(`{${f.name}}`).join(values[f.name] ?? "");
    onPrompt(text);
  };
  return (
    <Form className="space-y-3 rounded-[var(--radius-card)] border border-line bg-canvas/40 p-3" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      {b.title && <p className="text-sm font-medium text-fg">{b.title}</p>}
      {(b.fields ?? []).map((f) =>
        f.kind === "select" ? (
          <Select key={f.name} label={f.label} value={values[f.name] ?? ""} onChange={(v) => set(f.name, v)}
            options={(f.options ?? []).map((o) => ({ id: o, label: o }))} />
        ) : (
          <TextField key={f.name} label={f.label} multiline={f.kind === "textarea"} value={values[f.name] ?? ""} onChange={(v) => set(f.name, v)} />
        ),
      )}
      <Button type="submit" size="sm" variant="primary" tooltip="Send to the assistant">{b.label || "Submit"}</Button>
    </Form>
  );
}
