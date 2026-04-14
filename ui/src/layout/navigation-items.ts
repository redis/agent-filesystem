import type { IconType } from "@redislabsdev/redis-ui-icons";
import {
  CloudDownloadIcon,
  DashboardIcon,
  DatabaseIcon,
  DocumentationIcon,
  FoldersIcon,
  NotificationsIcon,
  RedisCopilotIcon,
  SupportIcon,
} from "@redislabsdev/redis-ui-icons";

export type SidebarPanelId = "root" | "workspaces";

export type NavigationRouteItem = {
  kind: "route";
  label: string;
  path: string;
  icon: IconType;
  title?: string;
};

export type NavigationPanelItem = {
  kind: "panel";
  label: string;
  icon: IconType;
  panelId: Exclude<SidebarPanelId, "root">;
  children: ReadonlyArray<NavigationRouteItem>;
};

export type NavigationItem = NavigationRouteItem | NavigationPanelItem;
export type NavigationTitleParts = {
  section?: string;
  page: string;
};

export const navigationItems: ReadonlyArray<NavigationItem> = [
  { kind: "route", label: "Overview", path: "/", icon: DashboardIcon },
  { kind: "route", label: "Workspaces", path: "/workspaces", icon: FoldersIcon },
  { kind: "route", label: "Databases", path: "/databases", icon: DatabaseIcon },
  { kind: "route", label: "Agents", path: "/agents", icon: RedisCopilotIcon },
  {
    kind: "route",
    label: "Activity",
    path: "/activity",
    icon: NotificationsIcon,
  },
];

export const bottomNavigationItems: ReadonlyArray<NavigationRouteItem> = [
  { kind: "route", label: "Downloads", path: "/downloads", icon: CloudDownloadIcon, title: "Downloads" },
  { kind: "route", label: "Docs", path: "/docs", icon: DocumentationIcon, title: "Documentation" },
  { kind: "route", label: "Agent Guide", path: "/agent-guide", icon: SupportIcon, title: "Agent Guide" },
];

function isPathMatch(pathname: string, path: string) {
  if (path === "/") {
    return pathname === "/";
  }

  return pathname.startsWith(path);
}

export function isNavigationItemActive(item: NavigationItem, pathname: string) {
  if (item.kind === "route") {
    return isPathMatch(pathname, item.path);
  }

  return item.children.some((child) => isPathMatch(pathname, child.path));
}

export function getSidebarPanelForPath(pathname: string): SidebarPanelId {
  const matchingPanel = navigationItems.find(
    (item) => item.kind === "panel" && isNavigationItemActive(item, pathname),
  );

  return matchingPanel?.kind === "panel" ? matchingPanel.panelId : "root";
}

export function resolveNavigationTitleParts(pathname: string): NavigationTitleParts {
  if (pathname.startsWith("/downloads")) {
    return { page: "Downloads" };
  }

  if (pathname.startsWith("/docs")) {
    return { page: "Documentation" };
  }

  if (pathname.startsWith("/agent-guide")) {
    return { page: "Agent Guide" };
  }

  if (pathname.startsWith("/databases")) {
    return { page: "Databases" };
  }

  if (pathname.startsWith("/workspaces/")) {
    return {
      section: "Workspaces",
      page: "Studio",
    };
  }

  if (pathname.startsWith("/agents")) {
    return { page: "Agents" };
  }

  for (const item of navigationItems) {
    if (item.kind === "route" && isPathMatch(pathname, item.path)) {
      return { page: item.title ?? item.label };
    }

    if (item.kind === "panel") {
      const match = item.children.find((child) => isPathMatch(pathname, child.path));
      if (match) {
        return {
          section: item.label,
          page: match.label,
        };
      }
    }
  }

  return { page: "" };
}

export function getNavigationPanel(_panelId: Exclude<SidebarPanelId, "root">) {
  return (
    navigationItems.find(
      (item): item is NavigationPanelItem => item.kind === "panel",
    ) ?? null
  );
}
