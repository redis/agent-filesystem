import { Button, Typography } from "@redis-ui/components";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import styled from "styled-components";
import { getControlPlaneURL } from "../foundation/api/afs";

/**
 * Top-of-page panel on /mcp. Purely informational — shows the single MCP
 * endpoint URL and the one-endpoint-many-tokens mental model. Token
 * management lives in the table below.
 */
export function MCPConnectionPanel() {
  const endpoint = useMemo(
    () => `${getControlPlaneURL().replace(/\/+$/, "")}/mcp`,
    [],
  );
  const [copied, setCopied] = useState(false);

  function copyEndpoint() {
    void navigator.clipboard.writeText(endpoint).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  }

  return (
    <Panel>
      <HeaderRow>
        <HeaderText>
          <Typography.Heading size="M">MCP Endpoint</Typography.Heading>
          <Lede>
            Every agent connects to the same URL. Tokens provide fine-grained
            access control.
          </Lede>
        </HeaderText>
        <HeaderActions>
          <Link to="/mcp/connect">
            <Button size="medium" variant="secondary-fill">
              How to connect
            </Button>
          </Link>
        </HeaderActions>
      </HeaderRow>

      <EndpointRow>
        <EndpointLabel>Endpoint</EndpointLabel>
        <EndpointValue>{endpoint}</EndpointValue>
        <Button size="small" variant="secondary-fill" onClick={copyEndpoint}>
          {copied ? "Copied!" : "Copy"}
        </Button>
      </EndpointRow>
    </Panel>
  );
}

/* ── Styled ── */

const Panel = styled.section`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background: var(--afs-panel-strong);
  padding: 24px 28px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
`;

const HeaderRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
`;

const HeaderText = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  max-width: 78ch;
`;

const HeaderActions = styled.div`
  flex-shrink: 0;
`;

const Lede = styled.p`
  margin: 2px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
`;

const EndpointRow = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: var(--afs-panel);
  overflow: hidden;

  @media (max-width: 700px) {
    flex-wrap: wrap;
  }
`;

const EndpointLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  flex-shrink: 0;
`;

const EndpointValue = styled.code`
  flex: 1;
  min-width: 0;
  overflow-x: auto;
  color: var(--afs-ink);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 13px;
  white-space: nowrap;
`;
