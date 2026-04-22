import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redis-ui/components";
import { useMemo } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
} from "../components/afs-kit";
import { useDatabaseScope } from "../foundation/database-scope";
import { computeChangelogTotals, formatChangelogBytes } from "../foundation/changelog-utils";
import { useActivity, useChangelog } from "../foundation/hooks/use-afs";
import { ActivityTable } from "../foundation/tables/activity-table";
import { ChangesTable } from "../foundation/tables/changes-table";
import type { AFSActivityEvent, AFSChangelogEntry } from "../foundation/types/afs";

const activitySearchSchema = z.object({
  view: z.enum(["changes", "events"]).optional(),
});

export const Route = createFileRoute("/activity")({
  validateSearch: activitySearchSchema,
  component: ActivityPage,
});

function ActivityPage() {
  const navigate = useNavigate();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const view = search.view ?? "changes";

  const activityQuery = useActivity(null, 50, view === "events");
  const changelogQuery = useChangelog(
    {
      limit: 1000,
      direction: "desc",
    },
    view === "changes",
  );

  const changelogEntries = changelogQuery.data?.entries ?? [];
  const changelogTotals = useMemo(
    () => computeChangelogTotals(changelogEntries),
    [changelogEntries],
  );
  const hasChangelogEntries = changelogEntries.length > 0;

  function setView(nextView: "changes" | "events") {
    void navigate({
      to: "/activity",
      search: nextView === "changes" ? {} : { view: nextView },
      replace: true,
    });
  }

  function openActivity(event: AFSActivityEvent) {
    if (event.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search: {
        ...(event.databaseId ? { databaseId: event.databaseId } : {}),
        ...(event.scope === "savepoint"
          ? { tab: "checkpoints" }
          : event.scope === "file"
            ? { tab: "browse" }
            : event.scope === "workspace"
              ? {}
              : { tab: "activity" }),
      },
    });
  }

  function openChange(entry: AFSChangelogEntry) {
    if (entry.workspaceId == null) {
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: entry.workspaceId },
      search: {
        ...(entry.databaseId ? { databaseId: entry.databaseId } : {}),
        tab: "changes",
      },
    });
  }

  return (
    <PageStack>
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            {view === "changes" ? "Changelog" : "Events"} below are partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}

      <FilterBar role="tablist" aria-label="Activity filters">
        <FilterButton
          type="button"
          role="tab"
          aria-selected={view === "changes"}
          $active={view === "changes"}
          onClick={() => setView("changes")}
        >
          Changelog
        </FilterButton>
        <FilterButton
          type="button"
          role="tab"
          aria-selected={view === "events"}
          $active={view === "events"}
          onClick={() => setView("events")}
        >
          Events
        </FilterButton>
      </FilterBar>

      {view === "changes" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <SectionHeader>
              <SectionTitle title="Changelog" />
              <HeaderSummary>
                {changelogQuery.isLoading ? (
                  "Loading changelog…"
                ) : hasChangelogEntries ? (
                  <>
                    <strong>{changelogTotals.added}</strong> added ·{" "}
                    <strong>{changelogTotals.modified}</strong> modified ·{" "}
                    <strong>{changelogTotals.deleted}</strong> deleted ·{" "}
                    <PositiveDelta>+{formatChangelogBytes(changelogTotals.bytesAdded)}</PositiveDelta>
                    {" / "}
                    <NegativeDelta>−{formatChangelogBytes(changelogTotals.bytesRemoved)}</NegativeDelta>
                  </>
                ) : (
                  "No changes yet"
                )}
              </HeaderSummary>
            </SectionHeader>
            <ChangesTable
              rows={changelogEntries}
              loading={changelogQuery.isLoading}
              error={changelogQuery.isError}
              errorMessage={
                changelogQuery.error instanceof Error
                  ? changelogQuery.error.message
                  : "Unable to load changes. Please retry."
              }
              emptyStateText="No changes have been recorded for any workspace yet."
              onOpenChange={openChange}
            />
          </SectionCard>
        </SectionGrid>
      ) : null}

      {view === "events" ? (
        activityQuery.isLoading ? (
          <Loader data-testid="loader--spinner" />
        ) : (
          <ActivityTable
            rows={activityQuery.data ?? []}
            loading={activityQuery.isLoading}
            error={activityQuery.isError}
            errorMessage={activityQuery.error instanceof Error ? activityQuery.error.message : undefined}
            onOpenActivity={openActivity}
          />
        )
      ) : null}
    </PageStack>
  );
}

const FilterBar = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  padding: 6px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 14px;
  background: var(--afs-panel, #fafafa);
`;

const FilterButton = styled.button<{ $active: boolean }>`
  border: none;
  background: ${({ $active }) => ($active ? "var(--afs-ink, #18181b)" : "transparent")};
  color: ${({ $active }) => ($active ? "#fff" : "var(--afs-muted, #71717a)")};
  padding: 9px 16px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  transition: background 140ms ease, color 140ms ease, transform 140ms ease;

  &:hover {
    color: ${({ $active }) => ($active ? "#fff" : "var(--afs-ink, #18181b)")};
    transform: translateY(-1px);
  }
`;

const HeaderSummary = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  white-space: nowrap;

  strong {
    color: var(--afs-ink);
    font-weight: 700;
  }
`;

const PositiveDelta = styled.span`
  color: #16a34a;
  font-weight: 700;
`;

const NegativeDelta = styled.span`
  color: #dc2626;
  font-weight: 700;
`;
