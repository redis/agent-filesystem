all: redis-fs-mount redis-fs-nfs

redis-fs-mount:
	go build -o redis-fs-mount ./cmd/redis-fs-mount

redis-fs-nfs:
	go build -o redis-fs-nfs ./cmd/redis-fs-nfs

clean:
	rm -f redis-fs-mount redis-fs-nfs

.PHONY: all clean redis-fs-mount redis-fs-nfs
