import { getControlPlaneURL } from "../../foundation/api/afs";
import type { Template } from "./templates-data";

export function templateInstallSlug(value: string, fallback = "workspace") {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/-{2,}/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || fallback;
}

export function templateServerName(workspaceName: string) {
  return `afs-${templateInstallSlug(workspaceName)}`;
}

export function templateSkillName(workspaceName: string) {
  return templateServerName(workspaceName);
}

export function templateToolServerName(workspaceName: string) {
  return templateServerName(workspaceName).replaceAll("-", "_");
}

export function templateToolPrefix(workspaceName: string) {
  return `mcp__${templateToolServerName(workspaceName)}__`;
}

export function templateMCPUrl(controlPlaneUrl = getControlPlaneURL()) {
  return `${controlPlaneUrl.replace(/\/+$/, "")}/mcp`;
}

export function substituteTemplateSkillPlaceholders(args: {
  text: string;
  workspaceName: string;
  template: Template;
}) {
  const serverName = templateServerName(args.workspaceName);
  const skillName = templateSkillName(args.workspaceName);
  const toolServerName = templateToolServerName(args.workspaceName);
  const toolPrefix = templateToolPrefix(args.workspaceName);
  return args.text
    .replaceAll("{{serverName}}", serverName)
    .replaceAll("{{skillName}}", skillName)
    .replaceAll("{{toolServerName}}", toolServerName)
    .replaceAll("{{toolPrefix}}", toolPrefix)
    .replaceAll("{{workspaceName}}", args.workspaceName)
    .replaceAll("{{templateSlug}}", args.template.slug);
}

export function buildTemplateSkillMarkdown(args: {
  workspaceName: string;
  template: Template;
}) {
  const skill = args.template.agentSkill;
  if (!skill) return null;
  const skillName = templateSkillName(args.workspaceName);
  const description = substituteTemplateSkillPlaceholders({
    text: skill.skillDescription,
    workspaceName: args.workspaceName,
    template: args.template,
  });
  const body = substituteTemplateSkillPlaceholders({
    text: skill.skillBody,
    workspaceName: args.workspaceName,
    template: args.template,
  }).trimEnd();

  return `---
name: ${skillName}
description: ${JSON.stringify(description)}
---

${body}
`;
}

export function buildSkillInstallCommand(args: {
  workspaceName: string;
  template: Template;
  skillsRoot: string;
}) {
  const markdown = buildTemplateSkillMarkdown(args);
  if (!markdown) return null;
  const skillName = templateSkillName(args.workspaceName);
  const marker = "AFS_SKILL_EOF";
  return `mkdir -p ${args.skillsRoot}/${skillName} && cat > ${args.skillsRoot}/${skillName}/SKILL.md <<'${marker}'
${markdown.trimEnd()}
${marker}`;
}

export function buildGenericAgentInstructions(args: {
  workspaceName: string;
  template: Template;
}) {
  const serverName = templateServerName(args.workspaceName);
  const toolPrefix = templateToolPrefix(args.workspaceName);
  const body = args.template.agentSkill
    ? substituteTemplateSkillPlaceholders({
        text: args.template.agentSkill.skillBody,
        workspaceName: args.workspaceName,
        template: args.template,
      }).trimEnd()
    : `Use the ${serverName} MCP server to read and write this Agent Filesystem workspace. Start by listing the workspace root, then follow the user's instructions.`;

  return `You have access to the Agent Filesystem workspace "${args.workspaceName}" through the MCP server "${serverName}". In Codex, hyphenated MCP server names usually appear as tools prefixed ${toolPrefix}*. In other clients, use the live tool namespace they expose.

${body}
`;
}
