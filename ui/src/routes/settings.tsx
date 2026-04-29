import { useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useClerk } from "@clerk/react";
import { Button, Loader, Typography } from "@redis-ui/components";
import styled from "styled-components";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogError,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  PageDescription,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../components/afs-kit";
import { useAuthSession } from "../foundation/auth-context";
import { accountQueryOptions, useAccount, useDeleteAccountMutation, useResetAccountDataMutation } from "../foundation/hooks/use-afs";
import { queryClient } from "../foundation/query-client";
import { useSkin, type Skin } from "../foundation/skin-context";

export const Route = createFileRoute("/settings")({
  loader: () =>
    queryClient.ensureQueryData({ ...accountQueryOptions(), revalidateIfStale: true }),
  component: SettingsPage,
});

type PendingAction = "delete" | "reset" | null;

function SettingsPage() {
  const auth = useAuthSession();

  if (!auth.supportsAccountAuth) {
    return (
      <PageStack>
        <SkinSwitcher />
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle
              title="Settings"
              body="AFS Cloud account settings are only available when this installation uses hosted account authentication."
            />
          </SectionHeader>
          <PageDescription>
            This installation is using {auth.config.provider || auth.config.mode} authentication, so
            sign-out and account deletion are managed outside the AFS web UI.
          </PageDescription>
        </SectionCard>
      </PageStack>
    );
  }

  return <ClerkSettingsPage />;
}

const SKIN_OPTIONS: ReadonlyArray<{ value: Skin; label: string; body: string }> = [
  {
    value: "classic",
    label: "Classic",
    body: "Today's Redis-UI styling — light surfaces, blue accent, rounded cards.",
  },
  {
    value: "situation-room",
    label: "Modern",
    body: "Mono-first canvas with a focused app shell, crisp spacing, and high-contrast accents. Work in progress.",
  },
];

function SkinSwitcher() {
  const { skin, setSkin, isSwitcherEnabled } = useSkin();

  if (!isSwitcherEnabled) return null;

  return (
    <SectionCard $span={12}>
      <SectionHeader>
        <SectionTitle
          title="UI skin (dev only)"
          body="Switch between visual skins for the AFS console. The setting is stored locally and is not exposed to end users."
        />
      </SectionHeader>
      <SkinGrid>
        {SKIN_OPTIONS.map((option) => {
          const active = skin === option.value;
          return (
            <SkinOption
              key={option.value}
              type="button"
              role="radio"
              aria-checked={active}
              $active={active}
              onClick={() => setSkin(option.value)}
            >
              <SkinOptionLabel>{option.label}</SkinOptionLabel>
              <SkinOptionBody>{option.body}</SkinOptionBody>
              {active ? <SkinOptionStatus>Active</SkinOptionStatus> : null}
            </SkinOption>
          );
        })}
      </SkinGrid>
    </SectionCard>
  );
}

function ClerkSettingsPage() {
  const auth = useAuthSession();
  const navigate = useNavigate();
  const clerk = useClerk();
  const accountQuery = useAccount(!auth.isLoading && auth.isAuthenticated);
  const resetMutation = useResetAccountDataMutation();
  const deleteMutation = useDeleteAccountMutation();
  const [pendingAction, setPendingAction] = useState<PendingAction>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const account = accountQuery.data;
  const isWorking = resetMutation.isPending || deleteMutation.isPending;

  async function redirectAfterSignOut(target: "/login" | "/signup") {
    try {
      await clerk.signOut({ redirectUrl: target });
    } catch {
      window.location.assign(target);
    }
  }

  async function confirmPendingAction() {
    if (pendingAction == null) {
      return;
    }

    try {
      setActionError(null);
      if (pendingAction === "reset") {
        await resetMutation.mutateAsync();
        setPendingAction(null);
        void navigate({ to: "/" });
        return;
      }
      await deleteMutation.mutateAsync();
      await redirectAfterSignOut("/signup");
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Unable to complete that action.");
    }
  }

  return (
    <PageStack>
      <SkinSwitcher />
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle
            title="Account settings"
            body="Manage your AFS Cloud account and the development reset flow used to test onboarding."
          />
        </SectionHeader>

        {accountQuery.isLoading ? (
          <CenteredLoader>
            <Loader data-testid="loader--settings-account" />
          </CenteredLoader>
        ) : accountQuery.error instanceof Error ? (
          <InlineError>{accountQuery.error.message}</InlineError>
        ) : (
          <InfoGrid>
            <InfoCard>
              <Label>Signed in as</Label>
              <Value>{auth.displayName}</Value>
              {auth.secondaryLabel ? <Meta>{auth.secondaryLabel}</Meta> : null}
            </InfoCard>
            <InfoCard>
              <Label>Owned cloud databases</Label>
              <Value>{account?.ownedDatabaseCount ?? 0}</Value>
              <Meta>These are deleted by reset and full account deletion.</Meta>
            </InfoCard>
            <InfoCard>
              <Label>Owned workspaces</Label>
              <Value>{account?.ownedWorkspaceCount ?? 0}</Value>
              <Meta>Workspace state follows the databases that belong to your account.</Meta>
            </InfoCard>
          </InfoGrid>
        )}
      </SectionCard>

      <SectionGrid>
        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              title="Developer tools"
              body="Use this when you want to re-run the first-login experience without recreating your Clerk account."
            />
          </SectionHeader>
          <ActionCard>
            <ActionCopy>
              <ActionTitle>Reset onboarding state</ActionTitle>
              <ActionBody>
                Clears your getting-started workspace and any account-owned cloud databases, then sends
                you back to Overview so you can start onboarding again.
              </ActionBody>
            </ActionCopy>
            <WarningButton
              size="medium"
              variant="secondary-fill"
              onClick={() => {
                setActionError(null);
                setPendingAction("reset");
              }}
              disabled={isWorking || accountQuery.isLoading}
            >
              Reset onboarding state
            </WarningButton>
          </ActionCard>
        </SectionCard>

        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              title="Danger zone"
              body="This permanently removes your AFS Cloud account data, then deletes the account itself."
            />
          </SectionHeader>
          <DangerZoneCard>
            <ActionCopy>
              <ActionTitle>Delete account</ActionTitle>
              <ActionBody>
                Permanently deletes your AFS Cloud data and then removes your account so you can sign up
                again from a blank slate.
              </ActionBody>
            </ActionCopy>
            <DangerButton
              size="medium"
              onClick={() => {
                setActionError(null);
                setPendingAction("delete");
              }}
              disabled={isWorking || accountQuery.isLoading || !account?.canDeleteIdentity}
            >
              Delete account
            </DangerButton>
          </DangerZoneCard>
          {!account?.canDeleteIdentity ? (
            <PageDescription>
              Full account deletion is only available when AFS Cloud is using Clerk-backed account auth.
            </PageDescription>
          ) : null}
        </SectionCard>
      </SectionGrid>

      {pendingAction ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="account-danger-dialog-title"
          onClick={() => {
            if (!isWorking) {
              setPendingAction(null);
            }
          }}
        >
          <ConfirmCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <div>
                <DialogTitle id="account-danger-dialog-title">
                  {pendingAction === "delete" ? "Delete this account?" : "Reset onboarding state?"}
                </DialogTitle>
                <DialogBody>
                  {pendingAction === "delete"
                    ? "This deletes your AFS Cloud data, removes the account, and signs you out. You will need to sign up again to come back."
                    : "This clears your getting-started workspace and any account-owned cloud databases, then returns you to Overview. You will stay signed in."}
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={() => {
                  if (!isWorking) {
                    setPendingAction(null);
                  }
                }}
              >
                ×
              </DialogCloseButton>
            </DialogHeader>

            <Checklist>
              <li>Owned databases deleted: {account?.ownedDatabaseCount ?? 0}</li>
              <li>Owned workspaces removed: {account?.ownedWorkspaceCount ?? 0}</li>
              <li>{pendingAction === "delete" ? "Your account will be deleted" : "Your account will stay active and signed in"}</li>
            </Checklist>

            {actionError ? <DialogError role="alert">{actionError}</DialogError> : null}

            <DialogActions style={{ justifyContent: "flex-end", marginTop: 20 }}>
              <Button
                variant="secondary-fill"
                size="medium"
                onClick={() => setPendingAction(null)}
                disabled={isWorking}
              >
                Cancel
              </Button>
              {pendingAction === "delete" ? (
                <DangerButton size="medium" onClick={() => void confirmPendingAction()} disabled={isWorking}>
                  {deleteMutation.isPending ? "Deleting..." : "Delete account"}
                </DangerButton>
              ) : (
                <WarningButton size="medium" onClick={() => void confirmPendingAction()} disabled={isWorking}>
                  {resetMutation.isPending ? "Resetting..." : "Reset onboarding state"}
                </WarningButton>
              )}
            </DialogActions>
          </ConfirmCard>
        </DialogOverlay>
      ) : null}
    </PageStack>
  );
}

const CenteredLoader = styled.div`
  min-height: 180px;
  display: grid;
  place-items: center;
`;

const InlineError = styled.p`
  margin: 0;
  color: #c2364a;
  font-size: 14px;
  line-height: 1.5;
`;

const InfoGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
`;

const InfoCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background: var(--afs-panel);
  padding: 18px 20px;
  display: grid;
  gap: 8px;
`;

const Label = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const Value = styled(Typography.Heading).attrs({
  component: "div",
  size: "S",
})`
  && {
    margin: 0;
  }
`;

const Meta = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const ActionCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  background: var(--afs-panel);
  padding: 20px;
  display: grid;
  gap: 18px;
`;

const DangerZoneCard = styled(ActionCard)`
  border-color: rgba(185, 28, 28, 0.24);
  background: linear-gradient(180deg, rgba(185, 28, 28, 0.06), rgba(185, 28, 28, 0.02));
`;

const ActionCopy = styled.div`
  display: grid;
  gap: 8px;
`;

const ActionTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
`;

const ActionBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;

const ConfirmCard = styled(DialogCard)`
  width: min(520px, 100%);
`;

const Checklist = styled.ul`
  margin: 0;
  padding-left: 18px;
  color: var(--afs-ink-soft);
  font-size: 14px;
  line-height: 1.7;
`;

const WarningButton = styled(Button)`
  && {
    align-self: flex-start;
    border-color: rgba(180, 83, 9, 0.28);
    background: rgba(245, 158, 11, 0.12);
    color: #9a3412;
  }

  &&:hover:not(:disabled) {
    background: rgba(245, 158, 11, 0.18);
    border-color: rgba(180, 83, 9, 0.4);
  }
`;

const DangerButton = styled(Button)`
  && {
    align-self: flex-start;
    border-color: #b91c1c;
    background: #b91c1c;
    color: #fff;
  }

  &&:hover:not(:disabled) {
    border-color: #991b1b;
    background: #991b1b;
  }
`;

const SkinGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const SkinOption = styled.button<{ $active: boolean }>`
  position: relative;
  border: 1px solid ${({ $active }) => ($active ? "var(--afs-accent)" : "var(--afs-line)")};
  background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "var(--afs-panel-strong)")};
  border-radius: 12px;
  padding: 16px 18px;
  text-align: left;
  cursor: pointer;
  display: grid;
  gap: 6px;
  font: inherit;
  color: var(--afs-ink);
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover {
    border-color: var(--afs-line-strong);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

const SkinOptionLabel = styled.span`
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink);
`;

const SkinOptionBody = styled.span`
  font-size: 13px;
  color: var(--afs-muted);
  line-height: 1.5;
`;

const SkinOptionStatus = styled.span`
  position: absolute;
  top: 12px;
  right: 14px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--afs-accent);
`;
