import { createFileRoute } from "@tanstack/react-router";
import {
  DocPage,
  DocHero,
  DocHeroTitle,
  DocHeroSub,
  DocSection,
  DocHeading,
  DocProse,
  CodeBlock,
  InlineCode,
  CalloutBox,
} from "../components/doc-kit";
import { getControlPlaneURL } from "../foundation/api/afs";

export const Route = createFileRoute("/downloads")({
  component: DownloadsPage,
});

function DownloadsPage() {
  const controlPlaneUrl = getControlPlaneURL();
  const downloadCmd = `curl -fsSL "${controlPlaneUrl}/v1/cli?os=$(uname -s)&arch=$(uname -m)" -o afs && chmod +x afs`;

  return (
    <DocPage>
      <DocHero>
        <DocHeroTitle>Download AFS CLI</DocHeroTitle>
        <DocHeroSub>
          Download the latest <InlineCode>afs</InlineCode> CLI binary from this
          control plane. The command auto-detects your OS and CPU architecture.
        </DocHeroSub>
      </DocHero>

      <DocSection>
        <DocHeading>Install</DocHeading>
        <DocProse>
          Run this command to download the <InlineCode>afs</InlineCode> CLI
          matching this control plane version:
        </DocProse>
        <CodeBlock>
          <code>{downloadCmd}</code>
        </CodeBlock>

        <CalloutBox $tone="tip">
          <DocProse>
            Move the binary to a directory on your PATH (e.g.{" "}
            <InlineCode>sudo mv afs /usr/local/bin/</InlineCode>) so you can run{" "}
            <InlineCode>afs</InlineCode> from anywhere.
          </DocProse>
        </CalloutBox>
      </DocSection>
    </DocPage>
  );
}
