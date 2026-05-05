const DEFAULT_REDIS_INSIGHT_URL = "http://localhost:5540";

export function redisInsightUrl() {
  return (
    String(import.meta.env.VITE_REDIS_INSIGHT_URL ?? "").trim() ||
    DEFAULT_REDIS_INSIGHT_URL
  );
}
