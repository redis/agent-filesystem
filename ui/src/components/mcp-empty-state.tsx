import { Button } from "@redis-ui/components";
import styled, { keyframes } from "styled-components";

type Props = {
  onAddMCP: () => void;
  onAddLocalMCP: () => void;
};

export function MCPEmptyState({ onAddMCP, onAddLocalMCP }: Props) {
  return (
    <Hero>
      <HeroInner>
        <HeroHeader>
          <Eyebrow>
            <EyebrowDot />
            Model Context Protocol
          </Eyebrow>
          <HeroTitle>
            Give your AI agents secure, scoped access to your workspace.
          </HeroTitle>
          <HeroLede>
            Spin up an MCP server in seconds. Connect Claude Desktop, Cursor,
            Windsurf — or any MCP-native client — with a token that expires,
            can be revoked, and logs every read and write.
          </HeroLede>
          <HeroActions>
            <Button size="large" onClick={onAddMCP}>
              Add MCP server
            </Button>
            <Button
              size="large"
              variant="secondary-fill"
              onClick={onAddLocalMCP}
            >
              Use local MCP
            </Button>
          </HeroActions>
          <HeroMeta>
            Takes under a minute. No credit card. Revoke anytime.
          </HeroMeta>
        </HeroHeader>

        <HeroVisual aria-hidden="true">
          <OrbitBackdrop />
          <OrbitRing $delay="0s" />
          <OrbitRing $delay="-4s" />
          <CentralGlyph>
            <svg
              width="44"
              height="44"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M9 2v6" />
              <path d="M15 2v6" />
              <path d="M6 8h12v5a6 6 0 0 1-12 0z" />
              <path d="M12 19v3" />
            </svg>
          </CentralGlyph>
          <ClientChip $top="14%" $left="10%">Claude</ClientChip>
          <ClientChip $top="22%" $left="78%">Cursor</ClientChip>
          <ClientChip $top="70%" $left="12%">Windsurf</ClientChip>
          <ClientChip $top="76%" $left="74%">VS Code</ClientChip>
        </HeroVisual>
      </HeroInner>

      <BenefitsGrid>
        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <rect x="3" y="11" width="18" height="10" rx="2" ry="2" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Scoped by token</BenefitTitle>
          <BenefitBody>
            Each server is bound to a single workspace and database. Agents
            can only reach what you explicitly grant.
          </BenefitBody>
        </Benefit>

        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M20 7h-9" />
              <path d="M14 17H5" />
              <circle cx="17" cy="17" r="3" />
              <circle cx="7" cy="7" r="3" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Fine-grained profiles</BenefitTitle>
          <BenefitBody>
            Read-only, read / write, or read / write with checkpoints. Pick
            the permission profile that fits the task.
          </BenefitBody>
        </Benefit>

        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Expiry & instant revoke</BenefitTitle>
          <BenefitBody>
            Set a lifetime up front, or kill access in one click. Stale
            tokens never linger past their usefulness.
          </BenefitBody>
        </Benefit>

        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
              <polyline points="14 2 14 8 20 8" />
              <line x1="16" y1="13" x2="8" y2="13" />
              <line x1="16" y1="17" x2="8" y2="17" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Full audit trail</BenefitTitle>
          <BenefitBody>
            Every tool call, read, and write is logged with the agent,
            workspace, and timestamp. Nothing happens off the record.
          </BenefitBody>
        </Benefit>

        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polyline points="16 18 22 12 16 6" />
              <polyline points="8 6 2 12 8 18" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Hosted or local</BenefitTitle>
          <BenefitBody>
            Run on our infrastructure for zero setup, or use the local MCP
            binary when you need everything on your own machine.
          </BenefitBody>
        </Benefit>

        <Benefit>
          <BenefitIcon>
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
            </svg>
          </BenefitIcon>
          <BenefitTitle>Works with every MCP client</BenefitTitle>
          <BenefitBody>
            If it speaks MCP, it connects. Claude Desktop, Cursor, Windsurf,
            VS Code — paste the config and go.
          </BenefitBody>
        </Benefit>
      </BenefitsGrid>

      <StepStrip>
        <StepStripTitle>How it works</StepStripTitle>
        <StepRow>
          <StepPill>
            <StepNum>1</StepNum>
            <StepText>
              <StepHeadline>Create a server</StepHeadline>
              <StepBody>
                Pick a workspace, database, and permission profile.
              </StepBody>
            </StepText>
          </StepPill>
          <StepConnector aria-hidden="true" />
          <StepPill>
            <StepNum>2</StepNum>
            <StepText>
              <StepHeadline>Copy the config</StepHeadline>
              <StepBody>
                We give you a ready-to-paste snippet for your agent.
              </StepBody>
            </StepText>
          </StepPill>
          <StepConnector aria-hidden="true" />
          <StepPill>
            <StepNum>3</StepNum>
            <StepText>
              <StepHeadline>Your agent gets to work</StepHeadline>
              <StepBody>
                Read, write, and checkpoint — with every action logged.
              </StepBody>
            </StepText>
          </StepPill>
        </StepRow>
      </StepStrip>

      <FooterCta>
        <FooterCtaText>
          <FooterCtaTitle>Ready to plug your agent in?</FooterCtaTitle>
          <FooterCtaBody>
            Create your first MCP server and connect any MCP-compatible
            client in under a minute.
          </FooterCtaBody>
        </FooterCtaText>
        <Button size="large" onClick={onAddMCP}>
          Add MCP server
        </Button>
      </FooterCta>
    </Hero>
  );
}

/* ── Styled components ── */

const fadeIn = keyframes`
  from { opacity: 0; transform: translateY(6px); }
  to   { opacity: 1; transform: translateY(0); }
`;

const orbit = keyframes`
  from { transform: translate(-50%, -50%) rotate(0deg); }
  to   { transform: translate(-50%, -50%) rotate(360deg); }
`;

const pulse = keyframes`
  0%, 100% { opacity: 0.7; transform: translate(-50%, -50%) scale(1); }
  50%      { opacity: 1;   transform: translate(-50%, -50%) scale(1.04); }
`;

const Hero = styled.div`
  display: flex;
  flex-direction: column;
  gap: 28px;
  animation: ${fadeIn} 260ms ease;
`;

const HeroInner = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
  gap: 28px;
  align-items: stretch;
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  background:
    radial-gradient(
      120% 80% at 100% 0%,
      color-mix(in srgb, var(--afs-accent, #2563eb) 10%, transparent) 0%,
      transparent 60%
    ),
    var(--afs-panel-strong);
  overflow: hidden;

  @media (max-width: 960px) {
    grid-template-columns: 1fr;
  }
`;

const HeroHeader = styled.div`
  padding: 44px 44px 40px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  min-width: 0;

  @media (max-width: 720px) {
    padding: 28px 24px;
  }
`;

const Eyebrow = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  align-self: flex-start;
  padding: 6px 12px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent);
  color: var(--afs-accent, #2563eb);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
`;

const EyebrowDot = styled.span`
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--afs-accent, #2563eb);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--afs-accent, #2563eb) 24%, transparent);
`;

const HeroTitle = styled.h1`
  margin: 0;
  color: var(--afs-ink);
  font-size: clamp(1.75rem, 2.6vw, 2.5rem);
  font-weight: 700;
  line-height: 1.12;
  letter-spacing: -0.025em;
  max-width: 28ch;
`;

const HeroLede = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 16px;
  line-height: 1.6;
  max-width: 52ch;
`;

const HeroActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 6px;
`;

const HeroMeta = styled.div`
  color: var(--afs-muted);
  font-size: 12.5px;
  letter-spacing: 0.02em;
`;

const HeroVisual = styled.div`
  position: relative;
  min-height: 320px;
  overflow: hidden;

  @media (max-width: 960px) {
    min-height: 240px;
    border-top: 1px solid var(--afs-line);
  }
`;

const OrbitBackdrop = styled.div`
  position: absolute;
  inset: 0;
  background:
    radial-gradient(
      60% 60% at 50% 50%,
      color-mix(in srgb, var(--afs-accent, #2563eb) 14%, transparent) 0%,
      transparent 70%
    );
`;

const OrbitRing = styled.div<{ $delay: string }>`
  position: absolute;
  top: 50%;
  left: 50%;
  width: 260px;
  height: 260px;
  border-radius: 50%;
  border: 1px dashed
    color-mix(in srgb, var(--afs-accent, #2563eb) 28%, transparent);
  animation: ${orbit} 22s linear infinite;
  animation-delay: ${({ $delay }) => $delay};
  transform: translate(-50%, -50%);

  &:nth-of-type(2) {
    width: 380px;
    height: 380px;
    border-color: color-mix(in srgb, var(--afs-accent, #2563eb) 16%, transparent);
  }
`;

const CentralGlyph = styled.div`
  position: absolute;
  top: 50%;
  left: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 88px;
  height: 88px;
  border-radius: 22px;
  background: var(--afs-panel-strong);
  border: 1px solid var(--afs-line);
  color: var(--afs-accent, #2563eb);
  box-shadow: 0 18px 40px
    color-mix(in srgb, var(--afs-accent, #2563eb) 22%, transparent);
  animation: ${pulse} 3.2s ease-in-out infinite;
`;

const ClientChip = styled.div<{ $top: string; $left: string }>`
  position: absolute;
  top: ${({ $top }) => $top};
  left: ${({ $left }) => $left};
  padding: 6px 12px;
  border-radius: 999px;
  background: var(--afs-panel-strong);
  border: 1px solid var(--afs-line);
  color: var(--afs-ink);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.01em;
  box-shadow: 0 6px 14px rgba(8, 6, 13, 0.08);
`;

const BenefitsGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 960px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;

const Benefit = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 20px;
  background: var(--afs-panel-strong);
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: border-color 160ms ease, transform 160ms ease;

  &:hover {
    border-color: var(--afs-line-strong);
    transform: translateY(-1px);
  }
`;

const BenefitIcon = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 10%, transparent);
  color: var(--afs-accent, #2563eb);
`;

const BenefitTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.005em;
`;

const BenefitBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.55;
`;

const StepStrip = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 24px 24px 22px;
  background: var(--afs-panel-strong);
`;

const StepStripTitle = styled.div`
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  margin-bottom: 14px;
`;

const StepRow = styled.div`
  display: grid;
  grid-template-columns: 1fr auto 1fr auto 1fr;
  gap: 12px;
  align-items: stretch;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const StepPill = styled.div`
  display: flex;
  gap: 14px;
  align-items: flex-start;
  padding: 14px 16px;
  border-radius: 14px;
  background: var(--afs-panel);
  border: 1px solid var(--afs-line);
`;

const StepNum = styled.div`
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--afs-accent, #2563eb);
  color: #fff;
  font-size: 13px;
  font-weight: 800;
`;

const StepText = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const StepHeadline = styled.div`
  color: var(--afs-ink);
  font-size: 13.5px;
  font-weight: 700;
`;

const StepBody = styled.div`
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.5;
`;

const StepConnector = styled.div`
  align-self: center;
  width: 28px;
  height: 1px;
  background: var(--afs-line);

  @media (max-width: 860px) {
    display: none;
  }
`;

const FooterCta = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 22px 24px;
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background:
    linear-gradient(
      100deg,
      color-mix(in srgb, var(--afs-accent, #2563eb) 10%, transparent) 0%,
      transparent 55%
    ),
    var(--afs-panel-strong);

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const FooterCtaText = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
`;

const FooterCtaTitle = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const FooterCtaBody = styled.div`
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
`;
