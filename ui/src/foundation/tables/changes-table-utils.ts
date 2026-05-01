export function truncateMiddlePath(path: string): string {
  const value = path.trim();
  if (value === "") {
    return value;
  }

  const hasLeadingSlash = value.startsWith("/");
  const hasTrailingSlash = value.endsWith("/") && value.length > 1;
  const segments = value.split("/").filter(Boolean);
  const leadingSegments = 1;
  const trailingSegments = 2;

  if (segments.length <= leadingSegments + trailingSegments) {
    return value;
  }

  const leading = segments.slice(0, leadingSegments).join("/");
  const trailing = segments.slice(-trailingSegments).join("/");
  const prefix = hasLeadingSlash ? "/" : "";
  const suffix = hasTrailingSlash ? "/" : "";

  return `${prefix}${leading}/.../${trailing}${suffix}`;
}
