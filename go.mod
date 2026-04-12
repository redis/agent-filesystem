module github.com/redis/agent-filesystem

go 1.22.2

require github.com/redis/go-redis/v9 v9.18.0

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/redis/agent-filesystem/mount v0.0.0
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace github.com/redis/agent-filesystem/mount => ./mount
