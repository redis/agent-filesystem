import { z } from "zod";

export const studioTabValues = ["browse", "checkpoints", "changes", "settings"] as const;
const studioTabSearchValues = ["browse", "checkpoints", "activity", "changes", "settings"] as const;

export type StudioTab = (typeof studioTabValues)[number];

export const studioTabSchema = z.enum(studioTabSearchValues);

export function normalizeStudioTab(tab: (typeof studioTabSearchValues)[number] | undefined): StudioTab {
  if (tab == null) {
    return "browse";
  }
  return tab === "activity" ? "changes" : tab;
}
