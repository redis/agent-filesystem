all: afs afs-server redis-qmd

afs:
	go build -o ../afs .

afs-server:
	go build -o ../afs-server ./cmd/afs-server

redis-qmd:
	go build -o ../redis-qmd ./cmd/redis-qmd

clean:
	rm -f ../afs ../afs-server ../redis-qmd

.PHONY: all clean afs afs-server redis-qmd
