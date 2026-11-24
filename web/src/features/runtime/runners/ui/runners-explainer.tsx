import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { CodeBlock } from "../../../../lib/design/components/code-block/code-block";

const example = `jobs:
  build:
    runs-on: shippin-<size>
    steps:
      - uses: actions/checkout@v4
      - run: make test`;

/** How a member asks for a runner, said from the job's side: the sizes a plan offers are the provider's to
 * set, so the page says what happens to a job rather than listing them. */
export function RunnersExplainer() {
  return (
    <Card>
      <CardHeader title="How build runners work" description="Shippin runs your GitHub Actions jobs on machines your provider operates." />
      <div className="grid gap-6 p-5 lg:grid-cols-2">
        <ol className="list-decimal space-y-3 pl-5 text-base text-fg-muted">
          <li>
            <span className="text-fg">Connect GitHub</span> with the Shippin GitHub App and choose the repositories it can reach.
          </li>
          <li>
            <span className="text-fg">Assign each repository</span> to a workspace, and optionally a project, and allow Shippin runners for
            it. An owner or admin does this.
          </li>
          <li>
            <span className="text-fg">Ask for a runner in the workflow</span> with{" "}
            <code className="font-mono text-sm text-fg">runs-on: shippin-</code> followed by a size your provider gave you, such as{" "}
            <code className="font-mono text-sm text-fg">shippin-2x</code>.
          </li>
          <li>
            <span className="text-fg">Each job gets its own fresh machine</span>, used for that job alone and removed when it ends. Runners
            are never shared between organizations.
          </li>
        </ol>
        <div className="space-y-4">
          <CodeBlock label="Workflow example" code={example} />
          <p className="text-sm text-fg-muted">
            Your plan decides which sizes you may use, how many jobs run at once and how many minutes a month are included. If a job asks
            for a size your plan doesn't include, or its repository isn't assigned, it shows under Jobs as{" "}
            <span className="font-medium text-danger">Refused</span> with the reason, and gets no runner. Ask your provider which sizes your
            plan offers.
          </p>
        </div>
      </div>
    </Card>
  );
}
