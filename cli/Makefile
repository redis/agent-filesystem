all: afs afs-server

afs:
	go build -o ../afs .

afs-server:
	go build -o ../afs-server ./cmd/afs-server

clean:
	rm -f ../afs ../afs-server

.PHONY: all clean afs afs-server
