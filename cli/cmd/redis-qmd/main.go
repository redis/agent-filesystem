package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rowantrollope/agent-filesystem/cli/qmd"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6379", "Redis host:port")
	db := flag.Int("db", 0, "Redis DB")
	key := flag.String("key", "", "agent-filesystem key name (required)")
	index := flag.String("index", "", "RediSearch index name (default: afs_idx:{<key>})")
	flag.Parse()

	if *key == "" {
		fmt.Fprintln(os.Stderr, "error: --key is required")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: redis-qmd --key <key> [flags] <command> [args...]")
		fmt.Fprintln(os.Stderr, "commands: doctor, index <create|rebuild|info>, search <text>, query <dsl>")
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: *addr,
		DB:   *db,
	})
	defer rdb.Close()

	client := qmd.NewClient(rdb, *key, *index)
	ctx := context.Background()

	cmd := args[0]
	switch cmd {
	case "doctor":
		checks, err := client.Doctor(ctx)
		for _, c := range checks {
			fmt.Println(c)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "index":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: index <create|rebuild|info>")
			os.Exit(1)
		}
		switch args[1] {
		case "create":
			if err := client.CreateIndex(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			fmt.Println("index created:", client.IndexName())
		case "rebuild":
			if err := client.RebuildIndex(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			fmt.Println("index rebuilt:", client.IndexName())
		case "info":
			info, err := client.IndexInfo(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			for k, v := range info {
				fmt.Printf("%s: %s\n", k, v)
			}
		default:
			fmt.Fprintln(os.Stderr, "unknown index subcommand:", args[1])
			os.Exit(1)
		}

	case "search":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: search <text>")
			os.Exit(1)
		}
		text := strings.Join(args[1:], " ")
		ftQuery := qmd.BuildSimpleSearchQuery(text)
		total, hits, err := client.Search(ctx, ftQuery, qmd.QueryOptions{Limit: 20})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("total: %d\n", total)
		for _, h := range hits {
			fmt.Printf("%.4f  %s  (%s, %d bytes)\n", h.Score, h.Path, h.Type, h.Size)
		}

	case "query":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: query <dsl>")
			os.Exit(1)
		}
		dslInput := strings.Join(args[1:], " ")
		parsed, err := qmd.ParseDSL(dslInput)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing query:", err)
			os.Exit(1)
		}
		total, hits, err := client.SearchParsed(ctx, parsed, qmd.QueryOptions{Limit: 20})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("total: %d\n", total)
		for _, h := range hits {
			fmt.Printf("%.4f  %s  (%s, %d bytes)\n", h.Score, h.Path, h.Type, h.Size)
		}

	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(1)
	}
}
