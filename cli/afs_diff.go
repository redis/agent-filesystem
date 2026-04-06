package main

import "sort"

type afsDiffEntry struct {
	Kind string
	Path string
}

func summarizeManifestDiff(base, local manifest) []afsDiffEntry {
	seen := make(map[string]struct{}, len(base.Entries)+len(local.Entries))
	paths := make([]string, 0, len(base.Entries)+len(local.Entries))
	for path := range base.Entries {
		if path == "/" {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range local.Entries {
		if path == "/" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)

	diff := make([]afsDiffEntry, 0)
	for _, path := range paths {
		baseEntry, inBase := base.Entries[path]
		localEntry, inLocal := local.Entries[path]
		switch {
		case !inBase && inLocal:
			diff = append(diff, afsDiffEntry{Kind: "A", Path: path})
		case inBase && !inLocal:
			diff = append(diff, afsDiffEntry{Kind: "D", Path: path})
		case !manifestEntryEquivalent(baseEntry, localEntry):
			diff = append(diff, afsDiffEntry{Kind: "M", Path: path})
		}
	}
	return diff
}
