import { useEffect, useState } from "react";
import type { Json } from "../../../lib/api/catalog-types";
import { Checkbox } from "../../../lib/design/components/checkbox/checkbox";
import { Select } from "../../../lib/design/components/select/select";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { JsonSchemaProperty } from "../../../lib/hooks/use-deployments";

type ConfigSchema = JsonSchemaProperty & { properties?: Record<string, JsonSchemaProperty>; required?: string[] };

export function ServiceConfigForm({ schema, value, onChange, onValidityChange }: {
  schema: ConfigSchema;
  value: Record<string, Json>;
  onChange: (value: Record<string, Json>) => void;
  onValidityChange: (valid: boolean) => void;
}) {
  const [invalidFields, setInvalidFields] = useState<Record<string, string>>({});
  const properties = schema.properties ?? {};
  const missing = missingRequired(schema, value);
  const valid = missing.length === 0 && Object.keys(invalidFields).length === 0;

  useEffect(() => onValidityChange(valid), [onValidityChange, valid]);

  const update = (key: string, next: Json | undefined) => {
    const updated = { ...value };
    if (next === undefined) delete updated[key];
    else updated[key] = next;
    onChange(updated);
  };

  return <div className="grid gap-3">
    {schema.description && <p className="text-base text-fg-muted">{schema.description}</p>}
    {Object.entries(properties).map(([key, property]) => <SchemaField key={key} name={key} path={key}
      schema={property} required={(schema.required ?? []).includes(key)} value={value[key]}
      onChange={(next) => update(key, next)} onValidityChange={(field, message) => setInvalidFields((current) => {
        if (message) return { ...current, [field]: message };
        const updated = { ...current }; delete updated[field]; return updated;
      })} />)}
    {missing.length > 0 && <p className="text-xs text-danger">Required fields are missing: {missing.join(", ")}</p>}
    {!valid && Object.entries(invalidFields).map(([field, message]) => <p key={field} className="text-xs text-danger">{field}: {message}</p>)}
  </div>;
}

function SchemaField({ name, path, schema, required, value, onChange, onValidityChange }: {
  name: string; path: string; schema: JsonSchemaProperty; required: boolean; value: Json | undefined;
  onChange: (value: Json | undefined) => void; onValidityChange: (field: string, message: string) => void;
}) {
  const label = `${schema.title || name}${required ? " *" : ""}`;
  const description = schema.description ?? (schema.format === "shippin-secret-ref"
    ? "Enter a Vaultix or SOPS reference. Secret values are not accepted." : undefined);

  if (schema.type === "object" || schema.properties) {
    const nested = isObject(value) ? value : {};
    return <fieldset className="grid gap-3 rounded-lg border border-line p-3">
      <legend className="px-1 text-base font-medium text-fg">{label}</legend>
      {description && <p className="text-xs text-fg-muted">{description}</p>}
      {Object.entries(schema.properties ?? {}).map(([key, child]) => <SchemaField key={`${path}.${key}`} name={key}
        path={`${path}.${key}`} schema={child} required={(schema.required ?? []).includes(key)} value={nested[key]}
        onChange={(next) => {
          const updated = { ...nested };
          if (next === undefined) delete updated[key]; else updated[key] = next;
          onChange(updated);
        }} onValidityChange={(field, message) => onValidityChange(field, message ? `${key}: ${message}` : "")} />)}
    </fieldset>;
  }

  if (schema.type === "boolean") {
    return <Checkbox isSelected={value === true} onChange={(selected) => onChange(selected)} description={description}>{label}</Checkbox>;
  }

  if (schema.enum?.length) {
    const unset = "__shippin_unset__";
    const options = [
      ...(!required ? [{ id: unset, label: "Not set" }] : []),
      ...schema.enum.map((item) => ({ id: JSON.stringify(item), label: typeof item === "string" ? item : JSON.stringify(item) })),
    ];
    const selected = value === undefined ? unset : JSON.stringify(value);
    return <Select label={label} options={options} value={selected} onChange={(id) => {
      if (id === unset) onChange(undefined);
      else {
        try { onChange(JSON.parse(id) as Json); } catch { onChange(undefined); }
      }
    }} />;
  }

  if (schema.type === "array") {
    return <ArrayField name={label} path={path} description={description} value={Array.isArray(value) ? value : []}
      onChange={onChange} onValidityChange={onValidityChange} />;
  }

  if (schema.type === "number" || schema.type === "integer") {
    return <TextField label={label} type="number" value={typeof value === "number" ? String(value) : ""} description={description}
      onChange={(text) => {
        if (!text.trim() && !required) { onChange(undefined); onValidityChange(path, ""); return; }
        const parsed = Number(text);
        if (!text.trim() || !Number.isFinite(parsed) || (schema.type === "integer" && !Number.isInteger(parsed))) {
          onValidityChange(path, schema.type === "integer" ? "Enter a whole number." : "Enter a valid number."); return;
        }
        onValidityChange(path, ""); onChange(parsed);
      }} />;
  }

  return <TextField label={label} value={typeof value === "string" ? value : ""} description={description}
    onChange={(text) => { onValidityChange(path, ""); onChange(text); }} />;
}

function ArrayField({ name, path, description, value, onChange, onValidityChange }: {
  name: string; path: string; description?: string; value: Json[];
  onChange: (value: Json[] | undefined) => void; onValidityChange: (field: string, message: string) => void;
}) {
  const serialized = JSON.stringify(value, null, 2);
  const [text, setText] = useState(serialized);
  useEffect(() => setText(serialized), [serialized]);
  return <TextField label={name} multiline mono value={text} description={description ?? "Enter a JSON array."}
    onChange={(next) => {
      setText(next);
      try {
        const parsed: unknown = JSON.parse(next);
        if (!Array.isArray(parsed)) throw new Error("Enter a JSON array.");
        onValidityChange(path, ""); onChange(parsed as Json[]);
      } catch (error) {
        onValidityChange(path, error instanceof Error ? error.message : `Invalid array at ${path}.`);
      }
    }} />;
}

function isObject(value: Json | undefined): value is Record<string, Json> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function missingRequired(schema: ConfigSchema, value: Record<string, Json>, parent = ""): string[] {
  const missing = (schema.required ?? []).filter((key) => value[key] === undefined).map((key) => parent ? `${parent}.${key}` : key);
  for (const [key, child] of Object.entries(schema.properties ?? {})) {
    if (child.properties && isObject(value[key])) missing.push(...missingRequired(child, value[key], parent ? `${parent}.${key}` : key));
  }
  return missing;
}
