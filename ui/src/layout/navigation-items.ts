import type { IconType } from "@redislabsdev/redis-ui-icons";
import {
  ClusterIcon,
  DashboardIcon,
  DatabaseIcon,
  NotificationsIcon,
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

const workspacePanelChildren: ReadonlyArray<NavigationRouteItem> = [
  {
    kind: "route",
    label: "Catalog",
    path: "/workspaces",
    icon: ClusterIcon,
    title: "Workspaces",
  },
  {
    kind: "route",
    label: "Sessions",
    path: "/sessions",
    icon: DatabaseIcon,
  },
];

export const navigationItems: ReadonlyArray<NavigationItem> = [
  { kind: "route", label: "Overview", path: "/", icon: DashboardIcon },
  {
    kind: "panel",
    label: "Workspaces",
    icon: ClusterIcon,
    panelId: "workspaces",
    children: workspacePanelChildren,
  },
  {
    kind: "route",
    label: "Activity",
    path: "/activity",
    icon: NotificationsIcon,
  },
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
  if (pathname.startsWith("/workspaces/")) {
    return {
      section: "Workspaces",
      page: "Studio",
    };
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
  return navigationItems.find(
    (item): item is NavigationPanelItem => item.kind === "panel",
  ) ?? null;
}
