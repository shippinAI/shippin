import { useState } from "react";
import { Form } from "react-aria-components";
import { Button } from "../../../lib/design/components/button/button";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useCreateProject } from "../../../lib/hooks/use-projects";

const pattern = "[a-z][a-z0-9\\-]*[a-z0-9]";

/** Blank project: a name and its first environment (the New project panel's first tab). */
export function CreateProjectForm({ tenantId }: { tenantId: string }) {
  const create = useCreateProject(tenantId);
  const [name, setName] = useState("");
  const [env, setEnv] = useState("dev");
  return (
    <Form
      className="space-y-4 p-5 pt-0"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate({ name, environment: env }, { onSuccess: () => setName("") });
      }}
    >
      <TextField label="Name" placeholder="my-project" value={name} onChange={setName} isRequired pattern={pattern} maxLength={40} description="Lowercase letters, digits and dashes." />
      <TextField label="First environment" value={env} onChange={setEnv} isRequired pattern={pattern} maxLength={24} />
      <Button type="submit" variant="primary" size="sm" isDisabled={create.isPending} tooltip="Create the project and its first environment">
        Create project
      </Button>
    </Form>
  );
}
