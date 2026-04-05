/*
 * path.c - Path manipulation utilities for Redis FS module.
 */

#include "redismodule.h"
#include "path.h"
#include <string.h>
#include <ctype.h>

#define FS_NORMALIZE_MAX_DEPTH 256

/* Normalize a path to absolute form. Resolves ".", "..", multiple slashes.
 * Always returns a path starting with '/'. */
char *fsNormalizePath(const char *path, size_t len) {
    // Stack of component start positions and lengths.
    struct { size_t start; size_t len; } parts[FS_NORMALIZE_MAX_DEPTH];
    int depth = 0;
    int overflow = 0;

    if (len == 0 || path[0] != '/') {
        // Treat relative path as absolute from root.
        // We'll prepend '/' conceptually.
    }

    size_t i = 0;
    while (i < len) {
        // Skip slashes.
        while (i < len && path[i] == '/') i++;
        if (i >= len) break;

        // Find component end.
        size_t start = i;
        while (i < len && path[i] != '/') i++;
        size_t clen = i - start;

        if (clen == 1 && path[start] == '.') {
            // Current dir - skip.
            continue;
        } else if (clen == 2 && path[start] == '.' && path[start+1] == '.') {
            // Parent dir.
            if (depth > 0) depth--;
        } else {
            if (depth < FS_NORMALIZE_MAX_DEPTH) {
                parts[depth].start = start;
                parts[depth].len = clen;
                depth++;
            } else {
                overflow = 1;
            }
        }
    }

    if (overflow) {
        return NULL;
    }

    if (depth == 0) {
        char *root = RedisModule_Alloc(2);
        root[0] = '/';
        root[1] = '\0';
        return root;
    }

    // Calculate total length.
    size_t total = 0;
    for (int j = 0; j < depth; j++) {
        total += 1 + parts[j].len; // '/' + component
    }

    char *result = RedisModule_Alloc(total + 1);
    size_t pos = 0;
    for (int j = 0; j < depth; j++) {
        result[pos++] = '/';
        memcpy(result + pos, path + parts[j].start, parts[j].len);
        pos += parts[j].len;
    }
    result[pos] = '\0';
    return result;
}

/* Return the parent directory. */
char *fsParentPath(const char *path, size_t len) {
    if (len <= 1) {
        // Root or empty → root.
        char *root = RedisModule_Alloc(2);
        root[0] = '/';
        root[1] = '\0';
        return root;
    }

    // Find the last '/'.
    size_t last = len - 1;
    // Skip trailing slash if any (shouldn't happen with normalized paths).
    if (path[last] == '/' && last > 0) last--;
    while (last > 0 && path[last] != '/') last--;

    if (last == 0) {
        char *root = RedisModule_Alloc(2);
        root[0] = '/';
        root[1] = '\0';
        return root;
    }

    char *result = RedisModule_Alloc(last + 1);
    memcpy(result, path, last);
    result[last] = '\0';
    return result;
}

/* Return the basename. */
char *fsBaseName(const char *path, size_t len) {
    if (len <= 1) {
        char *root = RedisModule_Alloc(2);
        root[0] = '/';
        root[1] = '\0';
        return root;
    }

    size_t end = len;
    // Skip trailing slash.
    if (path[end-1] == '/' && end > 1) end--;

    size_t start = end;
    while (start > 0 && path[start-1] != '/') start--;

    size_t blen = end - start;
    char *result = RedisModule_Alloc(blen + 1);
    memcpy(result, path + start, blen);
    result[blen] = '\0';
    return result;
}

/* Join two path components. */
char *fsJoinPath(const char *a, size_t alen, const char *b, size_t blen) {
    // If b is absolute, just normalize b.
    if (blen > 0 && b[0] == '/') {
        return fsNormalizePath(b, blen);
    }

    // Concatenate a + "/" + b, then normalize.
    size_t total = alen + 1 + blen;
    char *tmp = RedisModule_Alloc(total + 1);
    memcpy(tmp, a, alen);
    tmp[alen] = '/';
    memcpy(tmp + alen + 1, b, blen);
    tmp[total] = '\0';

    char *result = fsNormalizePath(tmp, total);
    RedisModule_Free(tmp);
    return result;
}

/* Check if path is root "/". */
int fsIsRoot(const char *path, size_t len) {
    return (len == 1 && path[0] == '/');
}

/*
 * Full glob pattern matching — supports *, ?, [...], [!...], and \ escaping.
 * Modeled after Redis's stringmatchlen() and POSIX fnmatch() semantics.
 *
 *   *        Match zero or more characters.
 *   ?        Match exactly one character.
 *   [abc]    Match one of a, b, or c.
 *   [a-z]    Match any character in range a through z (inclusive).
 *   [!abc]   Match any character NOT in the set (^ also accepted).
 *   \x       Match the literal character x (escaping wildcards).
 *
 * The nocase parameter controls case-insensitive matching.
 */
static int fsGlobMatchInternal(const char *pattern, const char *string, int nocase) {
    while (*pattern && *string) {
        switch (*pattern) {
        case '*':
            // Collapse consecutive stars.
            while (*pattern == '*') pattern++;
            if (*pattern == '\0') return 1;
            // Try matching the rest of the pattern at each position.
            while (*string) {
                if (fsGlobMatchInternal(pattern, string, nocase)) return 1;
                string++;
            }
            return fsGlobMatchInternal(pattern, string, nocase);

        case '?':
            // Match any single character.
            pattern++;
            string++;
            break;

        case '[': {
            // Character class.
            pattern++;
            int negate = 0;
            if (*pattern == '!' || *pattern == '^') {
                negate = 1;
                pattern++;
            }

            int matched = 0;
            unsigned char sc = (unsigned char)*string;
            if (nocase) sc = (unsigned char)tolower(sc);

            /* Empty class "[]" is not valid — treat ']' as first literal
             * if it appears immediately after '[' or '[!' */
            while (*pattern && *pattern != ']') {
                unsigned char lo, hi;

                if (*pattern == '\\' && *(pattern+1)) {
                    pattern++;
                    lo = (unsigned char)*pattern;
                } else {
                    lo = (unsigned char)*pattern;
                }
                if (nocase) lo = (unsigned char)tolower(lo);

                // Check for range: a-z
                if (*(pattern+1) == '-' && *(pattern+2) && *(pattern+2) != ']') {
                    pattern += 2; // skip past '-'
                    if (*pattern == '\\' && *(pattern+1)) {
                        pattern++;
                        hi = (unsigned char)*pattern;
                    } else {
                        hi = (unsigned char)*pattern;
                    }
                    if (nocase) hi = (unsigned char)tolower(hi);

                    if (lo <= hi) {
                        if (sc >= lo && sc <= hi) matched = 1;
                    } else {
                        // Reversed range, e.g., [z-a] — match if in either direction.
                        if (sc >= hi && sc <= lo) matched = 1;
                    }
                } else {
                    // Single character match.
                    if (sc == lo) matched = 1;
                }
                pattern++;
            }

            if (*pattern == ']') pattern++; // Skip closing bracket.

            if (negate) matched = !matched;
            if (!matched) return 0;
            string++;
            break;
        }

        case '\\':
            // Escape: next character is literal.
            pattern++;
            if (*pattern == '\0') return 0;
            // Fall through to literal comparison.
            // fallthrough

        default: {
            // Literal character comparison.
            unsigned char pc = (unsigned char)*pattern;
            unsigned char sc = (unsigned char)*string;
            if (nocase) {
                pc = (unsigned char)tolower(pc);
                sc = (unsigned char)tolower(sc);
            }
            if (pc != sc) return 0;
            pattern++;
            string++;
            break;
        }
        }
    }

    // Skip trailing stars.
    while (*pattern == '*') pattern++;
    return (*pattern == '\0' && *string == '\0');
}

int fsGlobMatch(const char *pattern, const char *string) {
    return fsGlobMatchInternal(pattern, string, 0);
}

int fsGlobMatchNoCase(const char *pattern, const char *string) {
    return fsGlobMatchInternal(pattern, string, 1);
}
