all: afs redis-qmd

afs:
	go build -o ../afs .

redis-qmd:
	go build -o ../redis-qmd ./cmd/redis-qmd

clean:
	rm -f ../afs ../redis-qmd

.PHONY: all clean afs redis-qmd
