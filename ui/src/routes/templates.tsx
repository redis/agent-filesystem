import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import styled from "styled-components";
import { CreateWorkspaceDialog } from "../features/workspaces/CreateWorkspaceDialog";
import { templates } from "../features/templates/templates-data";

export const Route = createFileRoute("/templates")({
  component: TemplatesPage,
});

const GALLERY_TEMPLATES = templates.filter((template) => template.id !== "blank");

function TemplatesPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <Wrap>
      <Section>
        <SectionHeading>Multi-agent workflows</SectionHeading>
        <SectionRule />
        <Grid>
          {GALLERY_TEMPLATES.map((template) => (
            <Card
              key={template.id}
              type="button"
              onClick={() => setSelectedId(template.id)}
              aria-label={`Use the ${template.title} template`}
            >
              <CardHead>
                <IconSlot $accent={template.accent}>
                  <template.icon size="M" />
                </IconSlot>
                <AddFab aria-hidden>+</AddFab>
              </CardHead>
              <CardTitle>{template.title}</CardTitle>
              <CardBody>{template.tagline}</CardBody>
              <ProfileBadge $profile={template.profile}>
                {template.profileLabel}
              </ProfileBadge>
            </Card>
          ))}
        </Grid>
      </Section>

      <CreateWorkspaceDialog
        open={selectedId != null}
        initialTemplateId={selectedId ?? undefined}
        onClose={() => setSelectedId(null)}
      />
    </Wrap>
  );
}

/* ── Styled components ── */

const Wrap = styled.div`
  display: flex;
  flex-direction: column;
  gap: 28px;
  width: min(100%, 1080px);
  margin: 0 auto;
  padding: 28px 32px 56px;

  @media (max-width: 900px) {
    padding: 20px 18px 40px;
  }
`;

const Section = styled.section`
  display: flex;
  flex-direction: column;
  gap: 14px;
`;

const SectionHeading = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.005em;
`;

const SectionRule = styled.div`
  height: 1px;
  background: var(--afs-line);
  margin: -6px 0 2px;
`;

const Grid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 780px) {
    grid-template-columns: 1fr;
  }
`;

const Card = styled.button`
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 18px 18px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  text-align: left;
  cursor: pointer;
  transition:
    transform 140ms ease,
    border-color 140ms ease,
    box-shadow 140ms ease;

  &:hover {
    border-color: var(--afs-accent, #2563eb);
    transform: translateY(-1px);
    box-shadow: 0 10px 24px rgba(8, 6, 13, 0.08);
  }

  &:hover [data-fab] {
    background: var(--afs-accent, #2563eb);
    color: #fff;
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #2563eb);
    outline-offset: 3px;
  }
`;

const CardHead = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
`;

const IconSlot = styled.div<{ $accent: string }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: ${({ $accent }) =>
    `color-mix(in srgb, ${$accent} 18%, transparent)`};
  color: ${({ $accent }) => $accent};
`;

const AddFab = styled.span.attrs({ "data-fab": true })`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-muted);
  font-size: 18px;
  font-weight: 600;
  line-height: 1;
  transition: background 140ms ease, color 140ms ease, border-color 140ms ease;
`;

const CardTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.005em;
`;

const CardBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.55;
`;

const ProfileBadge = styled.span<{ $profile: string }>`
  align-self: flex-start;
  margin-top: 2px;
  padding: 3px 9px;
  border-radius: 999px;
  font-size: 10.5px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: ${({ $profile }) => profileBackground($profile)};
  color: ${({ $profile }) => profileForeground($profile)};
`;

function profileBackground(profile: string) {
  switch (profile) {
    case "workspace-ro":
      return "color-mix(in srgb, #2563eb 14%, transparent)";
    case "workspace-rw-checkpoint":
      return "color-mix(in srgb, #22c55e 18%, transparent)";
    case "workspace-rw":
    default:
      return "color-mix(in srgb, #f59e0b 16%, transparent)";
  }
}

function profileForeground(profile: string) {
  switch (profile) {
    case "workspace-ro":
      return "#2563eb";
    case "workspace-rw-checkpoint":
      return "#16a34a";
    case "workspace-rw":
    default:
      return "#b45309";
  }
}
