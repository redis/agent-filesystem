import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import styled from "styled-components";
import { RedisLogoDarkFullIcon } from "@redis-ui/icons/multicolor";
import { useColorMode } from "../../foundation/theme-context";

type AuthShellProps = {
  title: string;
  subtitle: string;
  children: ReactNode;
};

export function AuthShell({ title, subtitle, children }: AuthShellProps) {
  const { colorMode, toggleColorMode } = useColorMode();

  return (
    <Page>
      <TopBar>
        <LogoLink to="/">
          <RedisLogoDarkFullIcon />
        </LogoLink>
        <ThemeToggle
          type="button"
          onClick={toggleColorMode}
          aria-label="Toggle theme"
          title="Toggle theme"
        >
          {colorMode === "dark" ? "☀" : "☾"}
        </ThemeToggle>
      </TopBar>

      <Content>
        <Inner>
          <Heading>
            <Title>{title}</Title>
            <Subtitle>{subtitle}</Subtitle>
          </Heading>
          <Slot>{children}</Slot>
        </Inner>
      </Content>

      <Footer>
        <FooterMeta>© {new Date().getFullYear()} Redis</FooterMeta>
        <FooterLinks>
          <FooterLink to="/docs">Docs</FooterLink>
          <FooterLinkAnchor href="https://redis.io/legal/privacy-policy/" target="_blank" rel="noreferrer">
            Privacy
          </FooterLinkAnchor>
          <FooterLinkAnchor href="https://redis.io/legal/redis-website-terms-of-use/" target="_blank" rel="noreferrer">
            Terms
          </FooterLinkAnchor>
        </FooterLinks>
      </Footer>
    </Page>
  );
}

const Page = styled.div`
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr auto;
  background: var(--afs-bg);
`;

const TopBar = styled.header`
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 28px 40px;

  @media (max-width: 640px) {
    padding: 20px 20px;
  }
`;

const LogoLink = styled(Link)`
  display: inline-flex;
  align-items: center;
  color: inherit;

  svg {
    height: 44px;
    width: auto;
  }
`;

const ThemeToggle = styled.button`
  width: 40px;
  height: 40px;
  border-radius: 999px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-ink);
  cursor: pointer;
  font-size: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease, border-color 0.15s ease;

  &:hover {
    background: var(--afs-panel-strong);
    border-color: var(--afs-line-strong);
  }
`;

const Content = styled.main`
  display: grid;
  place-items: start center;
  padding: 24px 24px 64px;
`;

const Inner = styled.div`
  width: 100%;
  max-width: 440px;
  display: grid;
  gap: 28px;
  margin-top: clamp(16px, 6vh, 72px);
`;

const Heading = styled.div`
  display: grid;
  gap: 10px;
  text-align: center;
`;

const Title = styled.h1`
  margin: 0;
  font-size: 28px;
  line-height: 1.15;
  letter-spacing: -0.02em;
  color: var(--afs-ink);
  font-weight: 700;
`;

const Subtitle = styled.p`
  margin: 0;
  font-size: 15px;
  line-height: 1.5;
  color: var(--afs-muted);
`;

const Slot = styled.div`
  display: grid;
  justify-items: center;
`;

const Footer = styled.footer`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 40px;
  font-size: 12px;
  color: var(--afs-muted);

  @media (max-width: 640px) {
    padding: 16px 20px;
    flex-direction: column;
    gap: 8px;
  }
`;

const FooterMeta = styled.span``;

const FooterLinks = styled.div`
  display: flex;
  gap: 20px;
`;

const FooterLink = styled(Link)`
  color: var(--afs-muted);
  &:hover { color: var(--afs-ink); }
`;

const FooterLinkAnchor = styled.a`
  color: var(--afs-muted);
  &:hover { color: var(--afs-ink); }
`;
