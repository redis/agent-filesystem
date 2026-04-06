package nfs

import "github.com/go-git/go-billy/v5"

func invalidateVerifiers(h Handler, fs billy.Filesystem, paths ...[]string) {
	invalidator, ok := h.(VerifierInvalidator)
	if !ok {
		return
	}

	seen := make(map[string]struct{}, len(paths))
	for _, parts := range paths {
		path := fs.Join(parts...)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		invalidator.InvalidateVerifier(path)
	}
}
