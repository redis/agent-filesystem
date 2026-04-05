all: raf redis-qmd

raf:
	go build -o ../raf .

redis-qmd:
	go build -o ../redis-qmd ./cmd/redis-qmd

clean:
	rm -f ../raf ../redis-qmd

.PHONY: all clean raf redis-qmd
