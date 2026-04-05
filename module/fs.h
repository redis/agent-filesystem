/*
 * fs.h - Redis FS module internal definitions.
 *
 * One Redis key = one filesystem. The entire filesystem lives under a
 * single key as a custom module type (fstype), backed by a flat hashmap
 * of absolute paths to inodes.
 */

#ifndef REDIS_FS_H
#define REDIS_FS_H

/* POSIX feature test macros — must come before any system headers. */
#define _POSIX_C_SOURCE 200809L
#define _DEFAULT_SOURCE

#include <stdio.h>
#include <stdint.h>
#include <strings.h>
#include <time.h>

#include "redismodule.h"

/* Inode types. */
#define FS_INODE_FILE    0
#define FS_INODE_DIR     1
#define FS_INODE_SYMLINK 2

/* Default modes. */
#define FS_DEFAULT_FILE_MODE    0644
#define FS_DEFAULT_DIR_MODE     0755
#define FS_DEFAULT_SYMLINK_MODE 0777

/* Limits. */
#define FS_MAX_PATH_DEPTH  256
#define FS_MAX_SYMLINK_DEPTH 40
#define FS_MAX_TREE_DEPTH  64

/* A single inode in the filesystem. */
typedef struct fsInode {
    uint8_t type;           /* FS_INODE_FILE, FS_INODE_DIR, FS_INODE_SYMLINK */
    uint16_t mode;          /* POSIX permission bits (e.g., 0755) */
    uint32_t uid;           /* User ID */
    uint32_t gid;           /* Group ID */
    int64_t ctime;          /* Creation time (milliseconds since epoch) */
    int64_t mtime;          /* Modification time */
    int64_t atime;          /* Access time */
    union {
        struct {
            char *data;     /* File content (binary-safe) */
            size_t size;    /* Content length */
        } file;
        struct {
            char **children;    /* Array of child basenames (not full paths) */
            size_t count;       /* Number of children */
            size_t capacity;    /* Allocated capacity */
        } dir;
        struct {
            char *target;   /* Symlink target path */
        } symlink;
    } payload;
} fsInode;

/* The filesystem object — one per Redis key. */
typedef struct fsObject {
    RedisModuleDict *inodes;    /* path (C string) → fsInode* */
    uint64_t file_count;        /* Number of files */
    uint64_t dir_count;         /* Number of directories */
    uint64_t symlink_count;     /* Number of symlinks */
    uint64_t total_data_size;   /* Total bytes of file content */
} fsObject;

/* Module type handle (set during OnLoad). */
extern RedisModuleType *FSType;

/* ---- Inode lifecycle ---- */

/* Create a new inode. Mode 0 means use default for the type. */
fsInode *fsInodeCreate(uint8_t type, uint16_t mode);

/* Free an inode and its payload. */
void fsInodeFree(fsInode *inode);

/* ---- Filesystem object lifecycle ---- */

/* Create a new empty filesystem object. */
fsObject *fsObjectCreate(void);

/* Free a filesystem object and all its inodes. */
void fsObjectFree(fsObject *fs);

/* ---- Inode helpers ---- */

/* Add a child name to a directory inode. */
void fsDirAddChild(fsInode *dir, const char *name, size_t namelen);

/* Remove a child name from a directory inode. Returns 1 if found, 0 otherwise. */
int fsDirRemoveChild(fsInode *dir, const char *name, size_t namelen);

/* Check if a directory contains a child with the given name. */
int fsDirHasChild(fsInode *dir, const char *name, size_t namelen);

/* Set file data (copies the data). */
void fsFileSetData(fsInode *inode, const char *data, size_t len);

/* Append data to a file inode. */
void fsFileAppendData(fsInode *inode, const char *data, size_t len);

/* ---- Lookup helpers ---- */

/* Look up an inode by path. Returns NULL if not found. */
fsInode *fsLookup(fsObject *fs, const char *path, size_t pathlen);

/* Resolve symlinks (up to FS_MAX_SYMLINK_DEPTH). Returns the resolved path
 * as a newly allocated string, or NULL on error (sets *err). */
char *fsResolvePath(fsObject *fs, const char *path, size_t pathlen, int *err);

/* ---- Time helper ---- */

static inline int64_t fsNowMs(void) {
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

/* ---- RDB persistence ---- */
void FSRdbSave(RedisModuleIO *rdb, void *value);
void *FSRdbLoad(RedisModuleIO *rdb, int encver);
void FSFree(void *value);
size_t FSMemUsage(const void *value);
void FSDigest(RedisModuleDigest *md, void *value);

#endif /* REDIS_FS_H */
