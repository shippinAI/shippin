import { Plus } from "lucide-react";
import { useState } from "react";
import { Dialog, DialogTrigger, Heading, Modal, ModalOverlay } from "react-aria-components";
import { Button } from "../../../lib/design/components/button/button";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useCreateTask } from "../use-hub";

/** "New task" dialog: a person drops work onto the board for agents to claim. */
export function NewTask() {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const create = useCreateTask();
  return (
    <DialogTrigger>
      <Button variant="primary" tooltip="Put a task on the board"><Plus className="size-4" aria-hidden /> New task</Button>
      <ModalOverlay isDismissable className="fixed inset-0 z-50 grid place-items-center bg-scrim p-4">
        <Modal className="w-full max-w-md rounded-2xl border border-line bg-surface p-5 shadow-e3">
          <Dialog className="outline-none">
            {({ close }) => (
              <form className="flex flex-col gap-3" onSubmit={(e) => {
                e.preventDefault();
                create.mutate({ title, body }, {
                  onSuccess: () => { setTitle(""); setBody(""); close(); toast.success("Task added", title); },
                  onError: (err) => toast.error("Could not add the task", err.message),
                });
              }}>
                <Heading slot="title" className="text-lg font-semibold text-fg">New task</Heading>
                <TextField label="Title" value={title} onChange={setTitle} isRequired />
                <TextField label="Details" multiline value={body} onChange={setBody} />
                <div className="mt-1 flex justify-end gap-2">
                  <Button variant="ghost" tooltip="Close without saving" onPress={close}>Cancel</Button>
                  <Button variant="primary" type="submit" tooltip="Add to Open" isDisabled={!title.trim() || create.isPending}>Add task</Button>
                </div>
              </form>
            )}
          </Dialog>
        </Modal>
      </ModalOverlay>
    </DialogTrigger>
  );
}
