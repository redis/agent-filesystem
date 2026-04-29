module github.com/redis/agent-filesystem/mount

go 1.22.2

require (
	github.com/go-git/go-billy/v5 v5.6.2
	github.com/hanwen/go-fuse/v2 v2.7.2
	github.com/redis/agent-filesystem v0.0.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/willscott/go-nfs v0.0.3
	github.com/willscott/go-nfs-client v0.0.0-20240104095149-b44639837b00
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clerk/clerk-sdk-go/v2 v2.4.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-jose/go-jose/v3 v3.0.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rasky/go-xdr v0.0.0-20170124162913-1a41d1a06c93 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.34.5 // indirect
)

replace github.com/willscott/go-nfs => ../third_party/go-nfs

replace github.com/redis/agent-filesystem => ..
