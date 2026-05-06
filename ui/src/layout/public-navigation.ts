import { BookOpen, CloudDownload, LifeBuoy } from "lucide-react";

export const publicNavItems = [
  { label: "Docs", path: "/docs", icon: BookOpen },
  { label: "Downloads", path: "/downloads", icon: CloudDownload },
  { label: "Agent Guide", path: "/agent-guide", icon: LifeBuoy },
] as const;

export const publicRepoLink = {
  label: "Repo",
  href: "https://github.com/redis/agent-filesystem",
} as const;
