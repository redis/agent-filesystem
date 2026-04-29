#!/usr/bin/env node

import { readdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const templatesRoot = path.join(repoRoot, "templates");
const outputPath = path.join(
  repoRoot,
  "ui/src/features/templates/templates.generated.ts",
);

const requiredManifestFields = [
  "id",
  "slug",
  "title",
  "tagline",
  "icon",
  "accent",
  "profile",
  "profileLabel",
  "summary",
  "whyItMatters",
  "firstPrompt",
];

async function pathExists(target) {
  try {
    await stat(target);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
}

async function walkFiles(rootDir, currentDir = rootDir) {
  if (!(await pathExists(rootDir))) return [];
  const entries = await readdir(currentDir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(currentDir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await walkFiles(rootDir, fullPath)));
    } else if (entry.isFile()) {
      files.push(fullPath);
    }
  }

  return files.sort((left, right) =>
    relativeTemplatePath(rootDir, left).localeCompare(
      relativeTemplatePath(rootDir, right),
    ),
  );
}

function relativeTemplatePath(rootDir, filePath) {
  return path.relative(rootDir, filePath).split(path.sep).join("/");
}

function parseFrontmatter(markdown, filePath) {
  if (!markdown.startsWith("---\n")) {
    throw new Error(`${filePath} must start with YAML frontmatter`);
  }

  const end = markdown.indexOf("\n---\n", 4);
  if (end === -1) {
    throw new Error(`${filePath} has unterminated YAML frontmatter`);
  }

  const yaml = markdown.slice(4, end);
  const body = markdown.slice(end + "\n---\n".length);
  const fields = {};

  for (const line of yaml.split("\n")) {
    if (!line.trim()) continue;
    const separator = line.indexOf(":");
    if (separator === -1) {
      throw new Error(`${filePath} has invalid frontmatter line: ${line}`);
    }
    const key = line.slice(0, separator).trim();
    const rawValue = line.slice(separator + 1).trim();
    fields[key] = parseScalar(rawValue);
  }

  return { fields, body };
}

function parseScalar(value) {
  if (value.startsWith('"') && value.endsWith('"')) {
    return JSON.parse(value);
  }
  if (value.startsWith("'") && value.endsWith("'")) {
    return value.slice(1, -1);
  }
  return value;
}

async function readJson(filePath) {
  return JSON.parse(await readFile(filePath, "utf8"));
}

function validateManifest(manifest, filePath) {
  for (const field of requiredManifestFields) {
    if (manifest[field] == null) {
      throw new Error(`${filePath} is missing required field ${field}`);
    }
  }
  if (!Array.isArray(manifest.summary)) {
    throw new Error(`${filePath} field summary must be an array`);
  }
}

async function loadTemplate(templateDir) {
  const manifestPath = path.join(templateDir, "manifest.json");
  const manifest = await readJson(manifestPath);
  validateManifest(manifest, manifestPath);

  const seedRoot = path.join(templateDir, "seed");
  const seedFiles = [];
  for (const filePath of await walkFiles(seedRoot)) {
    seedFiles.push({
      path: relativeTemplatePath(seedRoot, filePath),
      content: await readFile(filePath, "utf8"),
    });
  }

  const skillPath = path.join(templateDir, "skill/SKILL.md");
  let agentSkill;
  if (await pathExists(skillPath)) {
    const { fields, body } = parseFrontmatter(
      await readFile(skillPath, "utf8"),
      skillPath,
    );
    if (!fields.description) {
      throw new Error(`${skillPath} is missing description frontmatter`);
    }
    agentSkill = {
      skillDescription: fields.description,
      skillBody: body.trim() + "\n",
    };

    const commandsRoot = path.join(templateDir, "commands");
    const commandFiles = await walkFiles(commandsRoot);
    if (commandFiles.length > 0) {
      agentSkill.commands = [];
      for (const commandPath of commandFiles) {
        const name = path.basename(commandPath, ".md");
        agentSkill.commands.push({
          name,
          body: await readFile(commandPath, "utf8"),
        });
      }
    }
  }

  return {
    ...manifest,
    seedFiles,
    ...(agentSkill ? { agentSkill } : {}),
  };
}

async function main() {
  const entries = await readdir(templatesRoot, { withFileTypes: true });
  const templates = [];

  for (const entry of entries.sort((left, right) =>
    left.name.localeCompare(right.name),
  )) {
    if (!entry.isDirectory()) continue;
    templates.push(await loadTemplate(path.join(templatesRoot, entry.name)));
  }

  const byId = new Map(templates.map((template) => [template.id, template]));
  const orderedIds = [
    "shared-agent-memory",
    "shared-llm-wiki-karpathy",
    "org-coding-standards",
    "team-planning-board",
    "blank",
  ];
  const orderedTemplates = orderedIds.map((id) => {
    const template = byId.get(id);
    if (!template) {
      throw new Error(`Missing template directory for ${id}`);
    }
    byId.delete(id);
    return template;
  });

  if (byId.size > 0) {
    throw new Error(
      `Add new templates to orderedIds in ${path.relative(repoRoot, fileURLToPath(import.meta.url))}: ${[
        ...byId.keys(),
      ].join(", ")}`,
    );
  }

  const contents = `/* This file is generated by scripts/generate-template-data.mjs. Do not edit by hand. */\n\nexport const generatedTemplates = ${JSON.stringify(
    orderedTemplates,
    null,
    2,
  )} as const;\n`;

  await writeFile(outputPath, contents);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
