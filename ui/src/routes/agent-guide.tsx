import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import styled from "styled-components";

export const Route = createFileRoute("/agent-guide")({
  component: AgentGuidePage,
});

function AgentGuidePage() {
  const [md, setMd] = useState<string | null>(null);

  useEffect(() => {
    fetch("/agent-guide.md")
      .then((r) => r.text())
      .then(setMd)
      .catch(() => setMd("_Failed to load agent-guide.md_"));
  }, []);

  return (
    <Wrapper>
      <QuickStart>
        <strong>Quick start for agents:</strong> Point your agent at the raw
        guide and tell it what to do:
        <Pre>{`Read the Agent Filesystem guide at
https://github.com/redis/agent-filesystem/blob/main/ui/public/agent-guide.md
and set up a workspace called "my-project". Use the AFS MCP
server to create files, organize the project, and create a
checkpoint when you're done.`}</Pre>
      </QuickStart>

      {md === null ? (
        <Loading>Loading guide…</Loading>
      ) : (
        <MarkdownBody dangerouslySetInnerHTML={{ __html: renderMarkdown(md) }} />
      )}
    </Wrapper>
  );
}

/* ── Minimal markdown → HTML ── */

function renderMarkdown(src: string): string {
  // Remove HTML comments
  const lines = src.split("\n").filter((l) => !l.trim().startsWith("<!--"));
  let html = lines.join("\n");

  // 1. Extract fenced code blocks into placeholders (protects contents from
  //    further markdown processing like # headings inside code).
  const codeBlocks: string[] = [];
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_m, _lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push(
      `<pre><code>${esc(code.trimEnd())}</code></pre>`,
    );
    return `\n%%CODEBLOCK_${idx}%%\n`;
  });

  // 2. Inline code
  html = html.replace(/`([^`]+)`/g, "<code>$1</code>");

  // 3. Tables
  html = html.replace(
    /^(\|.+\|)\n(\|[-| :]+\|)\n((?:\|.+\|\n?)*)/gm,
    (_m, header: string, _sep: string, body: string) => {
      const hCells = header
        .split("|")
        .slice(1, -1)
        .map((c: string) => `<th>${c.trim()}</th>`)
        .join("");
      const rows = body
        .trim()
        .split("\n")
        .map((row: string) => {
          const cells = row
            .split("|")
            .slice(1, -1)
            .map((c: string) => `<td>${c.trim()}</td>`)
            .join("");
          return `<tr>${cells}</tr>`;
        })
        .join("\n");
      return `<table><thead><tr>${hCells}</tr></thead><tbody>${rows}</tbody></table>`;
    },
  );

  // 4. Headings
  html = html.replace(/^#### (.+)$/gm, "<h4>$1</h4>");
  html = html.replace(/^### (.+)$/gm, "<h3>$1</h3>");
  html = html.replace(/^## (.+)$/gm, "<h2>$1</h2>");
  html = html.replace(/^# (.+)$/gm, "<h1>$1</h1>");

  // 5. Bold / italic
  html = html.replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/\*(.+?)\*/g, "<em>$1</em>");

  // 6. Unordered lists
  html = html.replace(/^- (.+)$/gm, "<li>$1</li>");
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, "<ul>$1</ul>");

  // 7. Paragraphs — wrap non-tag, non-placeholder lines
  html = html.replace(
    /^(?!<[a-z/])(?!%%CODEBLOCK_)((?:.+\n?)+)/gm,
    (block) => `<p>${block.trim()}</p>`,
  );

  // 8. Clean up empty paragraphs
  html = html.replace(/<p>\s*<\/p>/g, "");

  // 9. Re-insert code blocks
  html = html.replace(/%%CODEBLOCK_(\d+)%%/g, (_m, idx) => codeBlocks[+idx]);

  return html;
}

function esc(s: string) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/* ── Styles ── */

const Wrapper = styled.div`
  max-width: 780px;
  margin: 0 auto;
  padding: 40px 24px 80px;
`;

const QuickStart = styled.div`
  background: var(--afs-panel);
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  padding: 16px 20px;
  margin-bottom: 32px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--afs-ink);
`;

const Pre = styled.pre`
  margin: 12px 0 0;
  padding: 14px 16px;
  background: #1e1e2e;
  color: #cdd6f4;
  border-radius: 6px;
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 13px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre-wrap;
`;

const Loading = styled.p`
  color: var(--afs-muted);
  font-size: 14px;
`;

const MarkdownBody = styled.div`
  color: var(--afs-ink);
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 13.5px;
  line-height: 1.7;

  h1 {
    font-size: 22px;
    font-weight: 700;
    margin: 40px 0 16px;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--afs-line);
  }

  h2 {
    font-size: 18px;
    font-weight: 700;
    margin: 32px 0 12px;
    padding-bottom: 6px;
    border-bottom: 1px solid var(--afs-line);
  }

  h3 {
    font-size: 15px;
    font-weight: 700;
    margin: 24px 0 8px;
  }

  h4 {
    font-size: 14px;
    font-weight: 700;
    margin: 20px 0 6px;
  }

  p {
    margin: 8px 0;
  }

  code {
    background: var(--afs-line);
    padding: 1px 5px;
    border-radius: 3px;
    font-size: 12.5px;
  }

  pre {
    background: #1e1e2e;
    color: #cdd6f4;
    padding: 14px 16px;
    border-radius: 6px;
    overflow-x: auto;
    margin: 12px 0;

    code {
      background: none;
      padding: 0;
      font-size: 12.5px;
    }
  }

  table {
    width: 100%;
    border-collapse: collapse;
    margin: 12px 0;
    font-size: 12.5px;
  }

  th,
  td {
    text-align: left;
    padding: 6px 10px;
    border: 1px solid var(--afs-line);
  }

  th {
    background: color-mix(in srgb, var(--afs-line) 50%, transparent);
    font-weight: 700;
  }

  ul {
    margin: 8px 0;
    padding-left: 20px;
  }

  li {
    margin: 4px 0;
  }

  strong {
    font-weight: 700;
  }
`;
