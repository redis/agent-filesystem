// GlobalDrawer + HelpButton — root-level glue for the per-page drawer.
//
// GlobalDrawer reads the current drawer state from DrawerContext and renders
// either the OnboardingDrawer (with workspace creation status) or the
// CommandsDrawer (with the page's registered command sections).
//
// HelpButton lives in the AppBar; clicking it opens whatever the current
// page registered, falling back to the onboarding drawer.

import { ChevronRight } from "lucide-react";
import styled, { keyframes } from "styled-components";
import { useDrawer } from "../foundation/drawer-context";
import {
  CommandsDrawer,
  OnboardingDrawer,
} from "./onboarding-drawer";
import type { OnboardingStatus } from "./onboarding-drawer";
import { useScopedWorkspaceSummaries } from "../foundation/database-scope";
import { useQuickstartMutation } from "../foundation/hooks/use-afs";

export function GlobalDrawer() {
  const { state, close } = useDrawer();
  const quickstartMutation = useQuickstartMutation();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const workspaces = workspacesQuery.data;
  const haveAnyWorkspace = workspaces.length > 0;

  if (!state) return null;

  if (state.kind === "commands") {
    return (
      <CommandsDrawer
        title={state.title}
        subline={state.subline}
        sections={state.sections}
        onClose={close}
      />
    );
  }

  // Onboarding kind — derive status from the quickstart mutation. If the user
  // already has any workspace, treat the drawer as "ready" without re-firing.
  const status: OnboardingStatus = quickstartMutation.isPending
    ? "creating"
    : quickstartMutation.isSuccess || haveAnyWorkspace
      ? "ready"
      : quickstartMutation.isError
        ? "error"
        : "idle";

  const errorMessage = quickstartMutation.isError
    ? quickstartMutation.error.message.includes("cannot connect")
      ? "Could not connect to Redis at localhost:6379. Start Redis or add a remote database, then retry."
      : quickstartMutation.error.message || "Something went wrong."
    : null;

  const workspaceName =
    quickstartMutation.data?.workspace.name ??
    workspaces.find((w) => w.name === "getting-started")?.name ??
    "getting-started";

  function handleRetry() {
    void quickstartMutation.mutateAsync({}).catch(() => undefined);
  }

  return (
    <OnboardingDrawer
      path={state.path}
      status={status}
      errorMessage={errorMessage}
      workspaceName={workspaceName}
      onClose={close}
      onRetry={handleRetry}
    />
  );
}

export function HelpButton() {
  const { open, pageHelp } = useDrawer();

  function handleOpen() {
    if (pageHelp) {
      open({ kind: "commands", ...pageHelp });
    } else {
      open({ kind: "onboarding", path: "agent" });
    }
  }

  const hint = pageHelp ? pageHelp.title : "Getting Started";

  return (
    <HelpButtonRoot
      type="button"
      onClick={handleOpen}
      aria-label={hint}
      title={hint}
    >
      <TerminalCursor aria-hidden>_</TerminalCursor>
      <ChevronRight size={16} strokeWidth={2.4} />
    </HelpButtonRoot>
  );
}

const cursorBlink = keyframes`
  0%, 49% { opacity: 1; }
  50%, 100% { opacity: 0.25; }
`;

const HelpButtonRoot = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px 6px 10px;
  border-radius: 8px;
  border: 1px solid #1f2937;
  background: #0d1117;
  color: #4ade80;
  cursor: pointer;
  transition: background 120ms ease, border-color 120ms ease, box-shadow 120ms ease;

  &:hover {
    border-color: #4ade80;
    background: #0a1f15;
    box-shadow: 0 0 0 3px rgba(74, 222, 128, 0.12);
  }

  &:focus-visible {
    outline: 2px solid #4ade80;
    outline-offset: 2px;
  }
`;

const TerminalCursor = styled.span`
  display: inline-flex;
  align-items: flex-end;
  width: 12px;
  height: 16px;
  color: #4ade80;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 18px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: 0;
  text-shadow: 0 0 6px rgba(74, 222, 128, 0.45);
  animation: ${cursorBlink} 1.1s steps(1) infinite;
`;
