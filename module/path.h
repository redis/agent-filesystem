/*
 * path.h - Path manipulation utilities for Redis FS module.
 *
 * All paths in the filesystem are absolute (start with '/').
 * Functions return newly allocated strings that must be freed by the caller
 * using RedisModule_Free().
 */

#ifndef REDIS_FS_PATH_H
#define REDIS_FS_PATH_H

#include <stddef.h>

/* Normalize a path: resolve ".", "..", collapse multiple slashes,
 * remove trailing slash (except for root "/").
 * Returns a newly allocated string, or NULL if path depth exceeds 256
 * components. */
char *fsNormalizePath(const char *path, size_t len);

/* Return the parent directory of the given path.
 * "/" → "/", "/a/b" → "/a", "/a" → "/" */
char *fsParentPath(const char *path, size_t len);

/* Return the basename (final component) of the given path.
 * "/a/b/c" → "c", "/" → "/" */
char *fsBaseName(const char *path, size_t len);

/* Join two path components. Result is normalized.
 * Returns a newly allocated string. */
char *fsJoinPath(const char *a, size_t alen, const char *b, size_t blen);

/* Check if path is root "/" */
int fsIsRoot(const char *path, size_t len);

/* Simple glob pattern matching (* and ? wildcards). */
int fsGlobMatch(const char *pattern, const char *string);

/* Match a pattern against a string, case-insensitive. */
int fsGlobMatchNoCase(const char *pattern, const char *string);

#endif /* REDIS_FS_PATH_H */
