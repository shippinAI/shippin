import { Plus, Upload } from "lucide-react";
import { useState } from "react";
import { FileTrigger } from "react-aria-components";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useAddSSHKey } from "../../../lib/hooks/use-ssh-keys";

const keyRe = /^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com) [A-Za-z0-9+/=]{40,800}( \S{0,100})?$/;

/** Paste or upload a public key. */
export function AddKeyForm() {
  const add = useAddSSHKey();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const k = key.trim();
  const valid = keyRe.test(k) && name.trim() !== "";
  const onFile = async (files: FileList | null) => {
    const f = files?.[0];
    if (!f) return;
    if (f.size > 16384) return toast.error("That file is too large to be a public key");
    const text = (await f.text()).trim();
    if (!keyRe.test(text)) return toast.error("Not a public key", "Pick the .pub file, never the private key.");
    setKey(text);
    if (!name) setName(text.split(" ")[2] ?? f.name.replace(/\.pub$/, ""));
    toast.info(`Loaded ${f.name}`);
  };
  return (
    <Card>
      <CardHeader title="Add a key" description="ed25519, RSA or ECDSA public keys." />
      <div className="space-y-4 p-5">
        <TextField label="Name" value={name} onChange={setName} placeholder="laptop" />
        <TextField label="Public key" value={key} onChange={setKey} multiline mono placeholder="ssh-ed25519 AAAA… you@laptop" />
        <div className="flex items-center justify-between gap-2">
          <FileTrigger acceptedFileTypes={[".pub", "text/plain"]} onSelect={onFile}>
            <Button variant="ghost" size="sm" tooltip="Load a .pub file from your computer">
              <Upload className="size-4" /> Upload .pub
            </Button>
          </FileTrigger>
          <Button
            variant="primary"
            size="sm"
            isDisabled={!valid || add.isPending}
            tooltip={valid ? "Save and install on your workspaces" : "Enter a name and a valid public key"}
            onPress={() => add.mutate({ name: name.trim(), public_key: k }, { onSuccess: () => { setName(""); setKey(""); } })}
          >
            <Plus className="size-4" /> Add key
          </Button>
        </div>
      </div>
    </Card>
  );
}
