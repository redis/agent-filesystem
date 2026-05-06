// FormatToggle — render the same data as code in any of: cli, mcp, json, curl,
// py, ts. uses native <details>/<summary> so each format works without JS.
//
// Adapted from agent-site/src/frame.tsx (FormatToggle), restyled with
// styled-components to match the rest of ui/.
//
// Drop into any detail page that wants a "do this from your terminal" panel:
//
//   <FormatToggle
//     request={{ method: 'GET', path: '/v1/workspaces/foo' }}
//     json={data}
//     cliCommand={['afs', 'ws', 'info', 'foo', '--json']}
//     pyCall={`afs.workspaces.get('foo')`}
//     tsCall={`await afs.workspaces.get('foo')`}
//   />

import { useState } from "react";
import styled from "styled-components";
import * as snip from "../lib/snippets";
import type { RequestShape } from "../lib/snippets";

export type FormatToggleProps = {
  request: RequestShape;
  json: unknown;
  toolCall?: { name: string; args: Record<string, unknown> };
  cliCommand?: string[];
  pyCall?: string;
  tsCall?: string;
  // initial format opened on mount; null = all closed.
  defaultFormat?: "cli" | "mcp" | "json" | "curl" | "py" | "ts" | null;
  label?: string;
};

type Item = {
  key: "cli" | "mcp" | "json" | "curl" | "py" | "ts";
  label: string;
  available: boolean;
  render: () => string;
};

export function FormatToggle(props: FormatToggleProps) {
  const {
    request,
    json,
    toolCall,
    cliCommand,
    pyCall,
    tsCall,
    defaultFormat = "cli",
    label = "do this from your terminal:",
  } = props;
  const [open, setOpen] = useState<string | null>(defaultFormat);

  // CLI first — this surface treats CLI as canonical and REST as transport.
  const items: Item[] = [
    { key: "cli",  label: "cli",  available: !!cliCommand,
      render: () => (cliCommand ? snip.cli(cliCommand) : "(no CLI form for this view)") },
    { key: "mcp",  label: "mcp",  available: !!toolCall,
      render: () => (toolCall ? snip.mcp(toolCall.name, toolCall.args) : "(no MCP tool for this view)") },
    { key: "json", label: "json", available: true,
      render: () => snip.json(json) },
    { key: "curl", label: "curl", available: true,
      render: () => snip.curl(request) },
    { key: "py",   label: "py",   available: !!pyCall,
      render: () => (pyCall ? snip.py(pyCall) : "(no python sdk form)") },
    { key: "ts",   label: "ts",   available: !!tsCall,
      render: () => (tsCall ? snip.ts(tsCall) : "(no typescript sdk form)") },
  ];

  return (
    <Wrap>
      <Header>
        <Label>{label}</Label>
        <Tabs>
          {items.map((item) => (
            <Tab
              key={item.key}
              type="button"
              role="tab"
              aria-selected={open === item.key}
              $active={open === item.key}
              disabled={!item.available}
              onClick={() => setOpen(open === item.key ? null : item.key)}
            >
              {item.label}
            </Tab>
          ))}
        </Tabs>
      </Header>
      {open ? (
        <Code>
          {items.find((i) => i.key === open)?.render() ?? ""}
        </Code>
      ) : null}
    </Wrap>
  );
}

const Wrap = styled.section`
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: var(--afs-bg-soft);
`;

const Header = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
`;

const Label = styled.span`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.10em;
  text-transform: uppercase;
`;

const Tabs = styled.div`
  display: flex;
  gap: 4px;
  margin-left: auto;
`;

const Tab = styled.button<{ $active?: boolean }>`
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 11px;
  letter-spacing: 0.06em;
  padding: 3px 9px;
  border-radius: 4px;
  border: 1px solid ${(p) => (p.$active ? "var(--afs-accent)" : "var(--afs-line)")};
  background: ${(p) => (p.$active ? "var(--afs-accent)" : "transparent")};
  color: ${(p) => (p.$active ? "var(--afs-ink-on-accent)" : "var(--afs-muted)")};
  cursor: pointer;

  &:hover:not(:disabled) {
    color: ${(p) => (p.$active ? "var(--afs-ink-on-accent)" : "var(--afs-accent)")};
    border-color: var(--afs-accent);
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
`;

const Code = styled.pre`
  margin: 0;
  padding: 10px 14px;
  background: var(--afs-panel-strong);
  border: 1px solid var(--afs-line);
  border-radius: 6px;
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--afs-ink);
  white-space: pre-wrap;
  overflow-x: auto;
`;
