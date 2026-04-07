package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rowantrollope/agent-filesystem/cli/internal/controlplane"
	"github.com/rowantrollope/agent-filesystem/cli/qmd"
)

type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string { return o.value }

func (o *optionalString) Set(v string) error {
	o.value = v
	o.set = true
	return nil
}

type optionalInt struct {
	value int
	set   bool
}

func (o *optionalInt) String() string {
	if !o.set {
		return ""
	}
	return fmt.Sprintf("%d", o.value)
}

func (o *optionalInt) Set(v string) error {
	var parsed int
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &parsed); err != nil {
		return err
	}
	o.value = parsed
	o.set = true
	return nil
}

type cliOptions struct {
	redisConfig controlplane.Config
	key         string
	index       string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, cmdArgs, err := parseCLI(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		printUsage(stderr)
		return 1
	}
	if len(cmdArgs) == 0 {
		printUsage(stderr)
		return 1
	}

	rdb := controlplane.NewRedisClient(opts.redisConfig, 8)
	defer rdb.Close()

	client := qmd.NewClient(rdb, opts.key, opts.index)
	ctx := context.Background()

	cmd := cmdArgs[0]
	switch cmd {
	case "doctor":
		checks, err := client.Doctor(ctx)
		for _, c := range checks {
			fmt.Fprintln(stdout, c)
		}
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}

	case "index":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(stderr, "usage: index <create|rebuild|info>")
			return 1
		}
		switch cmdArgs[1] {
		case "create":
			if err := client.CreateIndex(ctx); err != nil {
				fmt.Fprintln(stderr, "error:", err)
				return 1
			}
			fmt.Fprintln(stdout, "index created:", client.IndexName())
		case "rebuild":
			if err := client.RebuildIndex(ctx); err != nil {
				fmt.Fprintln(stderr, "error:", err)
				return 1
			}
			fmt.Fprintln(stdout, "index rebuilt:", client.IndexName())
		case "info":
			info, err := client.IndexInfo(ctx)
			if err != nil {
				fmt.Fprintln(stderr, "error:", err)
				return 1
			}
			for k, v := range info {
				fmt.Fprintf(stdout, "%s: %s\n", k, v)
			}
		default:
			fmt.Fprintln(stderr, "unknown index subcommand:", cmdArgs[1])
			return 1
		}

	case "search":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(stderr, "usage: search <text>")
			return 1
		}
		text := strings.Join(cmdArgs[1:], " ")
		ftQuery := qmd.BuildSimpleSearchQuery(text)
		total, hits, err := client.Search(ctx, ftQuery, qmd.QueryOptions{Limit: 20})
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stdout, "total: %d\n", total)
		for _, h := range hits {
			fmt.Fprintf(stdout, "%.4f  %s  (%s, %d bytes)\n", h.Score, h.Path, h.Type, h.Size)
		}

	case "query":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(stderr, "usage: query <dsl>")
			return 1
		}
		dslInput := strings.Join(cmdArgs[1:], " ")
		parsed, err := qmd.ParseDSL(dslInput)
		if err != nil {
			fmt.Fprintln(stderr, "error parsing query:", err)
			return 1
		}
		total, hits, err := client.SearchParsed(ctx, parsed, qmd.QueryOptions{Limit: 20})
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stdout, "total: %d\n", total)
		for _, h := range hits {
			fmt.Fprintf(stdout, "%.4f  %s  (%s, %d bytes)\n", h.Score, h.Path, h.Type, h.Size)
		}

	default:
		fmt.Fprintln(stderr, "unknown command:", cmd)
		return 1
	}

	return 0
}

func parseCLI(args []string) (cliOptions, []string, error) {
	var opts cliOptions

	fs := flag.NewFlagSet("redis-qmd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var configPath string
	var addr optionalString
	var db optionalInt
	var key optionalString
	var index optionalString

	fs.StringVar(&configPath, "config", "", "path to afs.config.json")
	fs.Var(&addr, "addr", "Redis host:port (default: afs.config.json redisAddr, else 127.0.0.1:6379)")
	fs.Var(&db, "db", "Redis DB (default: afs.config.json redisDB, else 0)")
	fs.Var(&key, "key", "agent-filesystem key name (default: afs.config.json redisKey)")
	fs.Var(&index, "index", "RediSearch index name (default: afs_idx:{<key>})")
	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}

	opts.redisConfig = controlplane.Config{
		RedisAddr: "127.0.0.1:6379",
		RedisDB:   0,
	}

	cfg, present, err := controlplane.LoadConfigWithPresence(configPath)
	if err != nil {
		return opts, nil, err
	}
	if present {
		opts.redisConfig = cfg
		opts.key = cfg.RedisKey
	}

	if addr.set {
		opts.redisConfig.RedisAddr = addr.value
	}
	if db.set {
		opts.redisConfig.RedisDB = db.value
	}
	if key.set {
		opts.key = key.value
	}
	if index.set {
		opts.index = index.value
	}

	if strings.TrimSpace(opts.key) == "" {
		return opts, nil, fmt.Errorf("--key is required when afs.config.json does not provide redisKey")
	}
	if opts.redisConfig.RedisDB < 0 {
		return opts, nil, fmt.Errorf("redis db must be >= 0")
	}

	return opts, fs.Args(), nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: redis-qmd [flags] <command> [args...]")
	fmt.Fprintln(w, "commands: doctor, index <create|rebuild|info>, search <text>, query <dsl>")
	flagDefaults := []struct {
		name string
		help string
	}{
		{"-config", "path to afs.config.json"},
		{"-addr", "Redis host:port (default: afs.config.json redisAddr, else 127.0.0.1:6379)"},
		{"-db", "Redis DB (default: afs.config.json redisDB, else 0)"},
		{"-key", "agent-filesystem key name (default: afs.config.json redisKey)"},
		{"-index", "RediSearch index name (default: afs_idx:{<key>})"},
	}
	fmt.Fprintln(w, "flags:")
	for _, row := range flagDefaults {
		fmt.Fprintf(w, "  %-8s %s\n", row.name, row.help)
	}
}
