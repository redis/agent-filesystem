module github.com/rowantrollope/agent-filesystem/cli

go 1.22.2

require github.com/redis/go-redis/v9 v9.18.0

require github.com/rowantrollope/agent-filesystem/mount v0.0.0

require (
	github.com/alicebob/miniredis/v2 v2.37.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/rowantrollope/agent-filesystem/mount => ../mount
