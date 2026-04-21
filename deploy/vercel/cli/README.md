# CLI bundle staging

`prod.sh` cross-compiles the AFS CLI for each supported target
(darwin/amd64, darwin/arm64, linux/amd64, linux/arm64) and drops the
binaries into this directory before Vercel builds `main.go`. The
`//go:embed all:cli` in `cli_embed.go` then bakes them into the server
binary, and `extractCLIBundle` unpacks them to `/tmp/afs-cli/<os>-<arch>/afs`
at startup so the `/v1/cli` resolver can serve them.

This file exists so the embed pattern matches even on clean developer
builds where no binaries have been staged.
