import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

const cwd = process.cwd();
const localNpmrcPath = join(cwd, ".npmrc");
const localNpmrc = existsSync(localNpmrcPath)
  ? readFileSync(localNpmrcPath, "utf8")
  : "";
const userConfig = process.env.npm_config_userconfig;
const userNpmrc =
  userConfig && existsSync(userConfig) ? readFileSync(userConfig, "utf8") : "";
const npmAuthToken = process.env.NPM_AUTH_TOKEN || process.env.NODE_AUTH_TOKEN;
const npmConfigToken = process.env.npm_config__authToken;
const hasScopedRegistry =
  localNpmrc.includes("@redislabsdev:registry=https://npm.pkg.github.com/redislabsdev") ||
  userNpmrc.includes("@redislabsdev:registry=https://npm.pkg.github.com/redislabsdev");
const hasGithubPackageToken =
  localNpmrc.includes("npm.pkg.github.com/:_authToken=") ||
  userNpmrc.includes("npm.pkg.github.com/:_authToken=") ||
  Boolean(npmAuthToken) ||
  Boolean(npmConfigToken);

if (!hasScopedRegistry || !hasGithubPackageToken) {
  console.error(`
Missing GitHub Packages auth for @redislabsdev dependencies.

This UI installs private packages from https://npm.pkg.github.com/redislabsdev.

Fix:
  1. Copy ui/.npmrc.example to ui/.npmrc
  2. Export a token that can read those packages:
     export NPM_AUTH_TOKEN=YOUR_TOKEN
  3. Re-run npm install

Example:
  cd ui
  cp .npmrc.example .npmrc
  export NPM_AUTH_TOKEN=YOUR_TOKEN
  npm install

If you already have a token configured globally in ~/.npmrc, make sure it includes
the @redislabsdev scope and an auth token for npm.pkg.github.com.
`.trim());
  process.exit(1);
}
