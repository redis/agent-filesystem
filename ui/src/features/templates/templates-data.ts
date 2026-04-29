import type { IconType } from "@redis-ui/icons";
import {
  BotIcon,
  BookOpenIcon,
  FoldersIcon,
  SparklesIcon,
} from "../../components/lucide-icons";
import type { AFSMCPProfile } from "../../foundation/types/afs";
import { generatedTemplates } from "./templates.generated";

export type TemplateSeedFile = {
  path: string;
  content: string;
};

/**
 * Client-neutral agent skill. The install UI renders this as a Claude Code
 * skill, a Codex skill, a Claude plugin bundle, or plain generic instructions.
 * Placeholders such as `{{serverName}}`, `{{skillName}}`, and `{{toolPrefix}}`
 * are substituted in `agent-install.ts`.
 */
export type TemplateAgentSkill = {
  skillDescription: string;
  skillBody: string;
  commands?: ReadonlyArray<{
    name: string;
    body: string;
  }>;
};

export type Template = {
  id: string;
  slug: string;
  title: string;
  tagline: string;
  icon: IconType;
  accent: string;
  profile: AFSMCPProfile;
  profileLabel: string;
  summary: readonly string[];
  whyItMatters: string;
  seedFiles: readonly TemplateSeedFile[];
  firstPrompt: string;
  agentSkill?: TemplateAgentSkill;
};

const templateIcons = {
  bot: BotIcon,
  "book-open": BookOpenIcon,
  folders: FoldersIcon,
  sparkles: SparklesIcon,
} satisfies Record<(typeof generatedTemplates)[number]["icon"], IconType>;

export const templates: readonly Template[] = generatedTemplates.map(
  (template) => ({
    ...template,
    icon: templateIcons[template.icon],
  }),
);

export function findTemplate(id: string): Template | undefined {
  return templates.find((template) => template.id === id);
}

/* -------------------------------------------------------------------------- */
/* Setup prompt generator                                                     */
/* -------------------------------------------------------------------------- */

export function buildSetupPrompt(template: Template, workspaceName: string) {
  if (template.seedFiles.length === 0) {
    return `You're connected to a fresh Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents. The workspace is currently empty.

Suggest three ways we could use this workspace based on what I'm working on. Wait for me to pick one before creating any files.`;
  }

  const intro = `I've connected you to an Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents.

Initialize this workspace as "${template.title}" — ${template.tagline}

Use file_write to create each of the files below exactly as shown. The content for each file is the block between the opening "<<<FILE: path>>>" marker and the matching "<<<END>>>" marker.`;

  const fileBlocks = template.seedFiles
    .map(
      (file) => `<<<FILE: ${file.path}>>>
${file.content.trimEnd()}
<<<END>>>`,
    )
    .join("\n\n");

  const outro = `Once every file is written, run file_list on the workspace root and on each subdirectory you created, then give me a one-paragraph summary of the layout.

From then on, follow the protocol in AGENTS.md for this and every future session pointed at this workspace. When the user is ready, suggest they try:

> ${template.firstPrompt}`;

  return `${intro}

${fileBlocks}

${outro}`;
}
