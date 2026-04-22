import { z } from "zod";

export const studioTabValues = ["browse", "checkpoints", "activity", "changes", "settings"] as const;

export type StudioTab = (typeof studioTabValues)[number];

export const studioTabSchema = z.enum(studioTabValues);
