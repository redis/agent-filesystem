import { Button } from "@redis-ui/components";
import { Link } from "@tanstack/react-router";
import styled from "styled-components";
import { DialogCloseButton, DialogOverlay } from "./afs-kit";
import { SurfaceCard } from "./card-shell";

type Props = {
  open: boolean;
  used: number;
  limit: number;
  onClose: () => void;
};

export function FreeTierLimitDialog({ open, used, limit, onClose }: Props) {
  if (!open) return null;

  return (
    <DialogOverlay
      role="dialog"
      aria-modal="true"
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <Shell onClick={(event) => event.stopPropagation()}>
        <Card>
          <CloseButton type="button" aria-label="Close" onClick={onClose}>
            ×
          </CloseButton>

          <Eyebrow>Free tier</Eyebrow>
          <Title>You've used all {limit} free workspaces</Title>
          <UsageChip>
            <ChipDot />
            {used} / {limit} used on AFS Cloud
          </UsageChip>

          <Body>
            The free tier includes {limit} workspaces on the shared AFS Cloud
            database. Add your own Redis database to keep creating
            workspaces &mdash; you can host as many as you like there.
          </Body>

          <Actions>
            <Button size="large" variant="secondary-fill" onClick={onClose}>
              Not now
            </Button>
            <Button as={Link} to="/databases" size="large" onClick={onClose}>
              Add your own database &rarr;
            </Button>
          </Actions>
        </Card>
      </Shell>
    </DialogOverlay>
  );
}

const Shell = styled.div`
  width: min(560px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
`;

const Card = styled(SurfaceCard)`
  position: relative;
  border-radius: 16px;
  padding: 36px 32px 28px;
  background: var(--afs-panel);
  border: 1px solid color-mix(in srgb, var(--afs-accent) 16%, var(--afs-line));

  @media (max-width: 720px) {
    padding: 26px 22px 22px;
  }
`;

const CloseButton = styled(DialogCloseButton)`
  position: absolute;
  top: 18px;
  right: 18px;
`;

const Eyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const Title = styled.h2`
  margin: 10px 0 14px;
  color: var(--afs-ink);
  font-size: clamp(24px, 3.4vw, 30px);
  line-height: 1.15;
  letter-spacing: -0.02em;
  padding-right: 32px;
`;

const UsageChip = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px 6px 10px;
  border-radius: 999px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 16px;
`;

const ChipDot = styled.span`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ef4444;
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.18);
`;

const Body = styled.p`
  margin: 0;
  max-width: 56ch;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.6;
`;

const Actions = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 26px;
  flex-wrap: wrap;
`;
