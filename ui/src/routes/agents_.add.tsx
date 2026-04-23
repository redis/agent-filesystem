import { createFileRoute, Link } from "@tanstack/react-router";
import styled from "styled-components";
import { PageStack } from "../components/afs-kit";
import { AgentSetupGuide } from "../features/agents/AgentSetupGuide";

export const Route = createFileRoute("/agents_/add")({
  component: AddAgentPage,
});

function AddAgentPage() {
  return (
    <PageStack>
      <Header>
        <BackLink to="/agents">&larr; Back to agents</BackLink>
        <Title>Connect a new agent</Title>
        <Subtitle>
          Follow the steps below to download the AFS CLI or register your agent
          via the Model Context Protocol. The MCP path is workspace-bound by
          default, and connected agents appear on the Agents page in real time.
        </Subtitle>
      </Header>
      <AgentSetupGuide />
    </PageStack>
  );
}

const Header = styled.div`
  max-width: 720px;
  padding: 0 0 4px;
  display: grid;
  gap: 8px;
`;

const BackLink = styled(Link)`
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 4px;
  align-self: start;

  &:hover {
    color: var(--afs-ink);
  }
`;

const Title = styled.h2`
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--afs-ink);
  letter-spacing: -0.01em;
`;

const Subtitle = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
  max-width: 560px;
`;
