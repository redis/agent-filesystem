import type { AFSChangelogEntry } from "./types/afs";

export type ChangelogTotals = {
  added: number;
  modified: number;
  deleted: number;
  bytesAdded: number;
  bytesRemoved: number;
};

export function computeChangelogTotals(entries: AFSChangelogEntry[]): ChangelogTotals {
  let added = 0;
  let modified = 0;
  let deleted = 0;
  let bytesAdded = 0;
  let bytesRemoved = 0;

  for (const entry of entries) {
    switch (entry.op) {
      case "put":
      case "symlink":
      case "mkdir":
        if (entry.prevHash) {
          modified += 1;
        } else {
          added += 1;
        }
        break;
      case "delete":
      case "rmdir":
        deleted += 1;
        break;
      case "chmod":
        modified += 1;
        break;
    }

    const delta = entry.deltaBytes ?? 0;
    if (delta > 0) bytesAdded += delta;
    if (delta < 0) bytesRemoved += -delta;
  }

  return { added, modified, deleted, bytesAdded, bytesRemoved };
}

export function formatChangelogBytes(n: number): string {
  if (n === 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}
