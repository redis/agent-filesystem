export const searchBenchmark = {
  title: "Redis Search grep benchmark",
  corpus: "4,000 markdown files, 31.5 MiB",
  environment: "Docker redis:8, macOS arm64, 5 measured rounds",
  artifactPath: "/tmp/afs-bench-md-redis-search-20260429-002108",
  metrics: [
    {
      name: "Rare literal",
      afs: "17.35 ms",
      grep: "371.74 ms",
      ripgrep: "37.99 ms",
      summary: "21x faster than BSD grep; 2.2x faster than rg",
    },
    {
      name: "Common literal",
      afs: "42.56 ms",
      grep: "381.71 ms",
      ripgrep: "41.10 ms",
      summary: "9x faster than BSD grep; near ripgrep speed",
    },
    {
      name: "Regex fallback",
      afs: "1078.74 ms",
      grep: "213.16 ms",
      ripgrep: "67.53 ms",
      summary: "Regex still uses the non-indexed path",
    },
  ],
} as const;
