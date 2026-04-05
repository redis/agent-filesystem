all: agent-filesystem-mount agent-filesystem-nfs

agent-filesystem-mount:
	go build -o agent-filesystem-mount ./cmd/agent-filesystem-mount

agent-filesystem-nfs:
	go build -o agent-filesystem-nfs ./cmd/agent-filesystem-nfs

clean:
	rm -f agent-filesystem-mount agent-filesystem-nfs

.PHONY: all clean agent-filesystem-mount agent-filesystem-nfs
