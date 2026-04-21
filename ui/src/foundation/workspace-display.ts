export function canonicalWorkspaceName(name: string) {
  const trimmed = name.trim();
  const lowered = trimmed.toLowerCase();
  if (lowered === "getting-started" || lowered.startsWith("getting-started-")) {
    return "getting-started";
  }
  return trimmed;
}

export function displayWorkspaceName(name: string) {
  const canonical = canonicalWorkspaceName(name);
  if (canonical === "getting-started") {
    return "Getting-started";
  }
  return canonical;
}
