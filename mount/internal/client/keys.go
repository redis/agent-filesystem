package client

import (
	"path"
	"strings"
)

type keyBuilder struct {
	fsKey string
}

func newKeyBuilder(fsKey string) keyBuilder {
	return keyBuilder{fsKey: fsKey}
}

func (k keyBuilder) inode(p string) string {
	return "rfs:{" + k.fsKey + "}:inode:" + p
}

func (k keyBuilder) children(p string) string {
	return "rfs:{" + k.fsKey + "}:children:" + p
}

func (k keyBuilder) info() string {
	return "rfs:{" + k.fsKey + "}:info"
}

func (k keyBuilder) inodePrefix() string {
	return "rfs:{" + k.fsKey + "}:inode:"
}

func (k keyBuilder) childrenPrefix() string {
	return "rfs:{" + k.fsKey + "}:children:"
}

func (k keyBuilder) scanPattern() string {
	return "rfs:{" + k.fsKey + "}:*"
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}
