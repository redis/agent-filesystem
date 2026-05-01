import { Link, useLocation } from "@tanstack/react-router";
import { RedisLogoDarkFullIcon } from "@redis-ui/icons/multicolor";
import { BookOpen, CloudDownload, GitBranch, LifeBuoy } from "lucide-react";
import type { ReactNode } from "react";
import styled from "styled-components";
import { ThemeModeToggle } from "../components/theme-mode-toggle";

const publicNavItems = [
  { label: "Docs", path: "/docs", icon: BookOpen },
  { label: "Downloads", path: "/downloads", icon: CloudDownload },
  { label: "Agent Guide", path: "/agent-guide", icon: LifeBuoy },
] as const;

export function PublicShell({ children }: { children: ReactNode }) {
  const location = useLocation();

  return (
    <PublicFrame>
      <PublicHeader>
        <BrandLink to="/">
          <RedisLogo>
            <RedisLogoDarkFullIcon />
          </RedisLogo>
          <BrandDivider />
          <BrandText>
            <BrandName>
              <BrandAccent>afs</BrandAccent>.cloud
            </BrandName>
            <BrandSub>Agent Filesystem</BrandSub>
          </BrandText>
        </BrandLink>

        <PublicNav aria-label="Public navigation">
          {publicNavItems.map((item) => {
            const Icon = item.icon;
            const active = item.path === "/docs"
              ? location.pathname === "/docs" || location.pathname.startsWith("/docs/")
              : location.pathname === item.path;
            return (
              <PublicNavLink key={item.path} to={item.path} $active={active}>
                <Icon size={15} strokeWidth={1.9} aria-hidden="true" />
                {item.label}
              </PublicNavLink>
            );
          })}
          <RepoLink href="https://github.com/redis/agent-filesystem" target="_blank" rel="noreferrer">
            <GitBranch size={15} strokeWidth={1.9} aria-hidden="true" />
            Repo
          </RepoLink>
        </PublicNav>

        <PublicActions>
          <ThemeModeToggle />
        </PublicActions>
      </PublicHeader>

      <PublicMain>{children}</PublicMain>
    </PublicFrame>
  );
}

const PublicFrame = styled.div`
  position: relative;
  z-index: 1;
  min-height: 100vh;
`;

const PublicHeader = styled.header`
  position: sticky;
  top: 0;
  z-index: 20;
  display: grid;
  grid-template-columns: minmax(240px, auto) minmax(0, 1fr) auto;
  align-items: center;
  gap: 0px;
  min-height: 76px;
  padding: 1px 32px;
  border-bottom: 1px solid var(--afs-line);
  background: color-mix(in srgb, var(--afs-bg-1) 92%, transparent);
  backdrop-filter: blur(14px);

  @media (max-width: 1060px) {
    grid-template-columns: 1fr auto;
  }

  @media (max-width: 760px) {
    position: relative;
    grid-template-columns: 1fr;
    gap: 14px;
    padding: 16px 18px;
  }
`;

const BrandLink = styled(Link)`
  display: inline-flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
  color: var(--afs-ink);
  text-decoration: none;
`;

const RedisLogo = styled.span`
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;

  svg {
    display: block;
    width: 96px;
    height: auto;
  }
`;

const BrandDivider = styled.span`
  width: 1px;
  height: 38px;
  background: var(--afs-line-strong);
`;

const BrandText = styled.span`
  display: grid;
  gap: 1px;
  min-width: 0;
`;

const BrandName = styled.span`
  color: var(--afs-ink);
  font-family: var(--afs-font-mono);
  font-size: 22px;
  font-weight: var(--afs-fw-medium);
  line-height: 1;
  letter-spacing: 0;
`;

const BrandAccent = styled.span`
  color: var(--afs-accent);
`;

const BrandSub = styled.span`
  color: var(--afs-ink-dim);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-xs);
  letter-spacing: var(--afs-tr-caps);
  text-transform: uppercase;
`;

const PublicNav = styled.nav`
  display: flex;
  justify-content: center;
  gap: 8px;
  min-width: 0;

  @media (max-width: 1060px) {
    order: 3;
    grid-column: 1 / -1;
    justify-content: flex-start;
    overflow-x: auto;
    padding-bottom: 2px;
  }
`;

const publicLinkBase = `
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 34px;
  border: 1px solid transparent;
  border-radius: var(--afs-r-2);
  padding: 7px 10px;
  color: var(--afs-ink-dim);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-lg);
  letter-spacing: 0;
  line-height: 1;
  text-decoration: none;
  white-space: nowrap;
  transition: background var(--afs-dur-fast) var(--afs-ease), border-color var(--afs-dur-fast) var(--afs-ease), color var(--afs-dur-fast) var(--afs-ease);

  &:hover {
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-2);
    color: var(--afs-ink);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

const PublicNavLink = styled(Link)<{ $active: boolean }>`
  ${publicLinkBase}
  ${({ $active }) => $active ? `
    border-color: var(--afs-line-strong);
    background: var(--afs-accent-soft);
    color: var(--afs-accent);
  ` : ""}
`;

const RepoLink = styled.a`
  ${publicLinkBase}
`;

const PublicActions = styled.div`
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;

  @media (max-width: 760px) {
    justify-content: flex-start;
    flex-wrap: wrap;
  }
`;

const PublicMain = styled.main`
  position: relative;
  z-index: 1;
`;
