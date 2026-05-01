export function isPublicMarketingPath(pathname: string) {
  return (
    pathname === "/" ||
    pathname === "/docs" ||
    pathname.startsWith("/docs/") ||
    pathname === "/downloads" ||
    pathname === "/agent-guide"
  );
}
