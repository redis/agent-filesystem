import { Link, useLocation } from "@tanstack/react-router";
import { Check, Copy } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import styled from "styled-components";
import { getControlPlaneURL } from "../../foundation/api/afs";
import { getSiteAgentDocument } from "./public-agent-documents";

export function SiteAgentPane() {
  const location = useLocation();
  const controlPlaneUrl = getControlPlaneURL();
  const siteOrigin = typeof window !== "undefined" ? window.location.origin : controlPlaneUrl;
  const document = useMemo(
    () => getSiteAgentDocument(location.pathname, {
      controlPlaneUrl,
      siteOrigin,
      search: location.searchStr,
    }),
    [controlPlaneUrl, location.pathname, location.searchStr, siteOrigin],
  );
  const [assetMarkdown, setAssetMarkdown] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<"idle" | "copied">("idle");

  useEffect(() => {
    let cancelled = false;

    if (document.assetPath == null) {
      setAssetMarkdown(null);
      return () => {
        cancelled = true;
      };
    }

    setAssetMarkdown(null);
    fetch(document.assetPath)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`failed to fetch ${document.assetPath}`);
        }
        return response.text();
      })
      .then((text) => {
        if (!cancelled) {
          setAssetMarkdown(text.trim());
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAssetMarkdown("_Failed to load the markdown asset for this page._");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [document.assetPath]);

  useEffect(() => {
    if (copyState !== "copied") return;
    const timeout = window.setTimeout(() => setCopyState("idle"), 1800);
    return () => window.clearTimeout(timeout);
  }, [copyState]);

  const markdown = document.assetPath == null
    ? document.markdown.trim()
    : `${document.markdown.trim()}

---

${assetMarkdown ?? "_Loading markdown asset..._"}`;

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(markdown);
      setCopyState("copied");
    } catch {
      setCopyState("idle");
    }
  }

  return (
    <AgentFrame>
      <AgentHeader>
        <AgentHeaderCopy>
          <AgentEyebrow>Agent view</AgentEyebrow>
          <AgentPath>{location.pathname}</AgentPath>
        </AgentHeaderCopy>
        <AgentActions>
          <CopyButton type="button" onClick={handleCopy}>
            {copyState === "copied" ? (
              <Check size={15} strokeWidth={2.1} aria-hidden="true" />
            ) : (
              <Copy size={15} strokeWidth={2.1} aria-hidden="true" />
            )}
            {copyState === "copied" ? "Copied" : "Copy markdown"}
          </CopyButton>
        </AgentActions>
      </AgentHeader>

      <AgentLead>
        Raw markdown optimized for copy, pasting into another agent, and quick
        inspection without visual chrome.
      </AgentLead>

      {document.resources != null && document.resources.length > 0 ? (
        <ResourceRow aria-label="Agent view resources">
          {document.resources.map((resource) => (
            <ResourceLink
              key={`${resource.label}:${resource.href}`}
              as={resource.href.startsWith("http") ? "a" : Link}
              href={resource.href.startsWith("http") ? resource.href : undefined}
              rel={resource.href.startsWith("http") ? "noreferrer" : undefined}
              target={resource.href.startsWith("http") ? "_blank" : undefined}
              to={resource.href.startsWith("http") ? undefined : resource.href}
            >
              {resource.label}
            </ResourceLink>
          ))}
        </ResourceRow>
      ) : null}

      <MarkdownWindow>
        <MarkdownSource aria-label={`${document.title} markdown source`}>
          {markdown}
        </MarkdownSource>
      </MarkdownWindow>
    </AgentFrame>
  );
}

const AgentFrame = styled.div`
  min-height: 100vh;
  padding: 28px 18px 96px;
  background:
    radial-gradient(circle at top, rgba(255, 255, 255, 0.06), transparent 38%),
    linear-gradient(180deg, #0a0a0c 0%, #111117 55%, #0d0d11 100%);
`;

const AgentHeader = styled.div`
  width: min(100%, 980px);
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

const AgentHeaderCopy = styled.div`
  display: grid;
  gap: 6px;
`;

const AgentEyebrow = styled.span`
  color: rgba(210, 214, 220, 0.82);
  font-family: var(--afs-font-mono);
  font-size: 11px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
`;

const AgentPath = styled.code`
  color: #f5f7fa;
  font-family: var(--afs-font-mono);
  font-size: 14px;
`;

const AgentActions = styled.div`
  display: flex;
  justify-content: flex-end;
`;

const CopyButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 999px;
  padding: 0 14px;
  background: rgba(255, 255, 255, 0.06);
  color: #f5f7fa;
  font-family: var(--afs-font-mono);
  font-size: 12px;
  cursor: pointer;
  transition: background 160ms ease, border-color 160ms ease, transform 160ms ease;

  &:hover {
    border-color: rgba(255, 255, 255, 0.28);
    background: rgba(255, 255, 255, 0.1);
    transform: translateY(-1px);
  }

  &:focus-visible {
    outline: 2px solid rgba(255, 255, 255, 0.52);
    outline-offset: 2px;
  }
`;

const AgentLead = styled.p`
  width: min(100%, 980px);
  margin: 18px auto 0;
  color: rgba(214, 219, 226, 0.76);
  font-family: var(--afs-font-mono);
  font-size: 14px;
  line-height: 1.75;
`;

const ResourceRow = styled.div`
  width: min(100%, 980px);
  margin: 18px auto 0;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
`;

const ResourceLink = styled.a`
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  padding: 0 12px;
  color: rgba(244, 247, 250, 0.88);
  font-family: var(--afs-font-mono);
  font-size: 12px;
  text-decoration: none;
  transition: background 160ms ease, border-color 160ms ease;

  &:hover {
    border-color: rgba(255, 255, 255, 0.24);
    background: rgba(255, 255, 255, 0.06);
  }
`;

const MarkdownWindow = styled.div`
  width: min(100%, 980px);
  margin: 20px auto 0;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 28px;
  background: rgba(5, 5, 8, 0.72);
  box-shadow:
    0 24px 80px rgba(0, 0, 0, 0.44),
    inset 0 1px 0 rgba(255, 255, 255, 0.04);
  overflow: hidden;
`;

const MarkdownSource = styled.pre`
  margin: 0;
  padding: 28px;
  overflow-x: auto;
  color: rgba(245, 247, 250, 0.92);
  font-family: var(--afs-font-mono);
  font-size: 14px;
  line-height: 1.78;
  letter-spacing: 0;
  white-space: pre-wrap;
  word-break: break-word;

  &::selection {
    background: rgba(255, 255, 255, 0.2);
  }

  @media (max-width: 720px) {
    padding: 20px;
    font-size: 13px;
  }
`;
