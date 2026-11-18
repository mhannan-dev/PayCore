import { Badge, EmptyHint, Panel } from "../../ui";

export function RequestPreview({ preview }: { preview: string | null }) {
  return (
    <Panel title="Request preview" description="Sample account numbers are placeholders, not API UUIDs.">
      {preview ? (
        <>
          <div className="preview-status" role="status"><Badge>Validated locally</Badge><span>Not submitted</span></div>
          <pre className="code-preview">{preview}</pre>
        </>
      ) : (
        <EmptyHint icon="arrows" title="Your next movement starts here." text="Complete the form to inspect the balanced request." />
      )}
    </Panel>
  );
}
