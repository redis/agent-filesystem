# Vercel Deployment Notes

This folder is the home for Vercel-specific deployment material for AFS.

It exists to keep the core product code host-neutral while AFS Cloud is being
prototyped on Vercel.

What belongs here:

- deployment shape notes
- onboarding flow design for the hosted product
- Vercel-specific env/config/runbook docs
- preview-deployment smoke-check instructions

What does not belong here:

- core control-plane business logic
- long-term Redis-hosted production assumptions
- `.vercel/` project-link metadata created by `vercel link`

Current docs:

- [deployment-shape.md](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/deployment-shape.md)
- [onboarding-flows.md](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/onboarding-flows.md)
- [auth-plan.md](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/auth-plan.md)

Current wrapper:

- [main.go](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/main.go) is the thin Vercel-specific control-plane entrypoint used for preview boot/build validation.
- [preview.sh](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/preview.sh) stages a temporary Vercel build root and deploys a preview with the repo-root Go module intact.

Preview workflow:

```bash
./deploy/vercel/preview.sh --stage-only
./deploy/vercel/preview.sh
./deploy/vercel/smoke.sh https://your-preview-url.vercel.app
```

Production workflow:

```bash
./deploy/vercel/prod.sh
./deploy/vercel/prod.sh --alias agent-filesystem.vercel.app
```

Notes:

- The script intentionally uses `npx --yes vercel@latest` so it does not collide with any local binary named `vercel`.
- If [deploy/vercel/.vercel/project.json](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/.vercel/project.json) exists locally, the script copies that link metadata into the temporary staging directory before deploy.
- If the project is not linked yet, pass `--scope <team> --project <name>` and the script will link the staging directory before deploying.
- [smoke.sh](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/smoke.sh) uses `vercel curl` so it can hit protected preview deployments without needing a public share link.
- [prod.sh](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/prod.sh) deploys the same staged build root to production and can optionally try to attach a production alias.
