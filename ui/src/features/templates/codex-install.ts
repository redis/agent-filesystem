import { getControlPlaneURL } from "../../foundation/api/afs";
import type { Template } from "./templates-data";
import {
  buildSkillInstallCommand,
  templateMCPUrl,
  templateServerName,
} from "./agent-install";

export function buildCodexTomlConfig(workspaceName: string, token: string) {
  const url = templateMCPUrl(getControlPlaneURL());
  const serverName = templateServerName(workspaceName);
  return `[mcp_servers.${serverName}]
url = "${url}"
http_headers = { Authorization = "Bearer ${token || "<token-not-returned>"}" }`;
}

export function buildCodexSkillInstallCommand(args: {
  workspaceName: string;
  template: Template;
}): string | null {
  if (!args.template.agentSkill) {
    return null;
  }
  return buildSkillInstallCommand({
    workspaceName: args.workspaceName,
    template: args.template,
    skillsRoot: "~/.agents/skills",
  });
}
