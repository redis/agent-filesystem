import type { IconType } from "@redis-ui/icons";
import {
  CloudDownloadIcon,
  DashboardIcon,
  DatabaseIcon,
  DocumentationIcon,
  FoldersIcon,
  NotificationsIcon,
  RedisCopilotIcon,
  SupportIcon,
} from "@redis-ui/icons";

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
  subtitle?: string;
};

export const navigationItems: ReadonlyArray<NavigationItem> = [
  { kind: "route", label: "Overview", path: "/", icon: DashboardIcon },
  { kind: "route", label: "Workspaces", path: "/workspaces", icon: FoldersIcon },
  { kind: "route", label: "Agents", path: "/agents", icon: RedisCopilotIcon },
  { kind: "route", label: "Databases", path: "/databases", icon: DatabaseIcon },
  {
    kind: "route",
    label: "Activity",
    path: "/activity",
    icon: NotificationsIcon,
  },
];

export const bottomNavigationItems: ReadonlyArray<NavigationRouteItem> = [
  { kind: "route", label: "Docs", path: "/docs", icon: DocumentationIcon, title: "Documentation" },
  { kind: "route", label: "Downloads", path: "/downloads", icon: CloudDownloadIcon, title: "Downloads" },
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
    return { page: "Databases", subtitle: "Manage the databases where workspaces are hosted." };
  }

  if (pathname.startsWith("/workspaces")) {
    return { page: "Workspaces", subtitle: "Manage workspaces. These are the filesystems your agents can access." };
  }

  if (pathname.startsWith("/agents")) {
    return { page: "Agents", subtitle: "View and manage connected agents." };
  }

  if (pathname.startsWith("/activity")) {
    return { page: "Activity", subtitle: "Track workspace changes, agent actions, and system events." };
  }

  if (pathname.startsWith("/settings")) {
    return { page: "Settings", subtitle: "Manage your AFS Cloud account and developer reset options." };
  }

  for (const item of navigationItems) {
    if (item.kind === "route" && isPathMatch(pathname, item.path)) {
      if (item.path === "/") {
        return { page: item.title ?? item.label, subtitle: "Dashboard overview of workspaces, agents, and storage." };
      }
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
