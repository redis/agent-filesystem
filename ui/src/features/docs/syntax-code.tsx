import type { ReactNode } from "react";
import styled from "styled-components";

export type CodeLanguage = "typescript" | "python";

type SyntaxTone = "comment" | "string" | "keyword" | "function" | "number" | "constant";

const typeScriptKeywords = new Set([
  "async",
  "await",
  "catch",
  "class",
  "const",
  "export",
  "finally",
  "from",
  "if",
  "import",
  "interface",
  "let",
  "new",
  "return",
  "throw",
  "try",
  "type",
]);

const pythonKeywords = new Set([
  "as",
  "class",
  "def",
  "elif",
  "else",
  "finally",
  "for",
  "from",
  "if",
  "import",
  "in",
  "is",
  "not",
  "return",
  "try",
  "with",
]);

const constants = new Set([
  "AFS",
  "False",
  "None",
  "True",
  "console",
  "false",
  "null",
  "os",
  "process",
  "true",
  "undefined",
]);

const toneColor: Record<SyntaxTone, string> = {
  comment: "#64748b",
  string: "#047857",
  keyword: "#6d28d9",
  function: "#0b6bcb",
  number: "#b45309",
  constant: "#a21caf",
};

export function HighlightedCode(props: { code: string; language: CodeLanguage }) {
  return <>{highlightCode(props.code, props.language)}</>;
}

function highlightCode(code: string, language: CodeLanguage) {
  const nodes: ReactNode[] = [];

  code.split("\n").forEach((line, lineIndex) => {
    if (lineIndex > 0) {
      nodes.push("\n");
    }
    nodes.push(...highlightLine(line, language, `l${lineIndex}`));
  });

  return nodes;
}

function highlightLine(line: string, language: CodeLanguage, keyPrefix: string) {
  const nodes: ReactNode[] = [];
  let index = 0;
  let plainStart = 0;

  const flushPlain = (end: number) => {
    if (end > plainStart) {
      nodes.push(...highlightPlain(line.slice(plainStart, end), language, `${keyPrefix}-p${nodes.length}`));
    }
  };

  while (index < line.length) {
    if (isCommentStart(line, index, language)) {
      flushPlain(index);
      nodes.push(
        <SyntaxToken key={`${keyPrefix}-comment-${index}`} $tone="comment">
          {line.slice(index)}
        </SyntaxToken>,
      );
      return nodes;
    }

    const quote = line[index];
    if (quote === `"` || quote === `'` || (language === "typescript" && quote === "`")) {
      flushPlain(index);
      const stringEnd = scanString(line, index, quote, language);
      nodes.push(
        <SyntaxToken key={`${keyPrefix}-string-${index}`} $tone="string">
          {line.slice(index, stringEnd)}
        </SyntaxToken>,
      );
      index = stringEnd;
      plainStart = index;
      continue;
    }

    index += 1;
  }

  flushPlain(line.length);
  return nodes.length > 0 ? nodes : [line];
}

function highlightPlain(text: string, language: CodeLanguage, keyPrefix: string) {
  const nodes: ReactNode[] = [];
  const tokenPattern = /[A-Za-z_$][\w$]*|\d+(?:\.\d+)?/g;
  let cursor = 0;
  let match: RegExpExecArray | null;
  let tokenIndex = 0;

  while ((match = tokenPattern.exec(text)) !== null) {
    const raw = match[0];
    if (match.index > cursor) {
      nodes.push(text.slice(cursor, match.index));
    }

    const tone = tokenTone(raw, text.slice(tokenPattern.lastIndex), language);
    if (tone) {
      nodes.push(
        <SyntaxToken key={`${keyPrefix}-token-${tokenIndex}`} $tone={tone}>
          {raw}
        </SyntaxToken>,
      );
    } else {
      nodes.push(raw);
    }

    cursor = tokenPattern.lastIndex;
    tokenIndex += 1;
  }

  if (cursor < text.length) {
    nodes.push(text.slice(cursor));
  }

  return nodes;
}

function tokenTone(token: string, rest: string, language: CodeLanguage): SyntaxTone | null {
  const keywordSet = language === "typescript" ? typeScriptKeywords : pythonKeywords;

  if (keywordSet.has(token)) {
    return "keyword";
  }
  if (constants.has(token)) {
    return "constant";
  }
  if (/^\d/.test(token)) {
    return "number";
  }
  if (/^\s*\(/.test(rest)) {
    return "function";
  }

  return null;
}

function isCommentStart(line: string, index: number, language: CodeLanguage) {
  if (language === "python") {
    return line[index] === "#";
  }
  return line[index] === "/" && line[index + 1] === "/";
}

function scanString(line: string, start: number, quote: string, language: CodeLanguage) {
  if (
    language === "python" &&
    (quote === `"` || quote === `'`) &&
    line.slice(start, start + 3) === quote.repeat(3)
  ) {
    const tripleEnd = line.indexOf(quote.repeat(3), start + 3);
    return tripleEnd === -1 ? line.length : tripleEnd + 3;
  }

  let index = start + 1;
  while (index < line.length) {
    if (line[index] === quote && !isEscaped(line, index)) {
      return index + 1;
    }
    index += 1;
  }
  return line.length;
}

function isEscaped(line: string, index: number) {
  let slashCount = 0;
  let cursor = index - 1;

  while (cursor >= 0 && line[cursor] === "\\") {
    slashCount += 1;
    cursor -= 1;
  }

  return slashCount % 2 === 1;
}

const SyntaxToken = styled.span<{ $tone: SyntaxTone }>`
  color: ${({ $tone }) => toneColor[$tone]};
  font-weight: ${({ $tone }) => ($tone === "keyword" || $tone === "function" ? 700 : 500)};
`;
