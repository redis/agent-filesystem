import { z } from "zod";

export const studioTabValues = ["browse", "checkpoints", "activity", "settings"] as const;

export type StudioTab = (typeof studioTabValues)[number];

export function normalizeStudioTab(tab: StudioTab | "files"): StudioTab {
  return tab === "files" ? "browse" : tab;
}

export const studioTabSchema = z
  .union([z.enum(studioTabValues), z.literal("files")])
  .transform(normalizeStudioTab);
