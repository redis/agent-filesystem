import { z } from "zod";

export const studioTabValues = ["browse", "search", "checkpoints", "changes", "settings"] as const;
const studioTabSearchValues = ["browse", "search", "checkpoints", "activity", "changes", "settings"] as const;

export type StudioTab = (typeof studioTabSearchValues)[number];

export const studioTabSchema = z.enum(studioTabSearchValues);

export function normalizeStudioTab(tab: (typeof studioTabSearchValues)[number] | undefined): StudioTab {
  if (tab == null) {
    return "browse";
  }
  return tab === "activity" ? "changes" : tab;
}
