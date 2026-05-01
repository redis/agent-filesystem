import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import styled, { createGlobalStyle } from "styled-components";
import { RedisLogoDarkFullIcon } from "@redis-ui/icons/multicolor";
import { ThemeModeToggle } from "../../components/theme-mode-toggle";

/**
 * @redis-ui/styles normalizes the root font-size to 62.5% (10px) so that
 * rem-based design tokens resolve cleanly. Clerk's prebuilt components use
 * rem/em internally and render crushed at that scale. Restore the browser
 * default on auth routes so Clerk's own spacing, padding, and fonts look
 * right without us needing to micro-manage every Clerk element.
 */
const AuthRootFontSize = createGlobalStyle`
  html, :root { font-size: 16px; }
  body { font-size: 14px; }
`;

type AuthShellProps = {
  title: string;
  subtitle: string;
  children: ReactNode;
};

export function AuthShell({ title, subtitle, children }: AuthShellProps) {
  return (
    <Page>
      <AuthRootFontSize />
      <BrandPanel>
        <BrandInner>
          <BrandLogo>
            <RedisLogoDarkFullIcon />
          </BrandLogo>

          <BrandBody>
            <Headline>
              a filesystem for <s>humans</s> agents
            </Headline>
          </BrandBody>
        </BrandInner>
      </BrandPanel>

      <FormPanel>
        <FormTopBar>
          <MobileLogoLink to="/">
            <RedisLogoDarkFullIcon />
          </MobileLogoLink>
          <AuthThemeToggle />
        </FormTopBar>

        <FormBody>
          <FormInner>
            <Heading>
              <Title>{title}</Title>
              <Subtitle>{subtitle}</Subtitle>
            </Heading>
            <Slot>{children}</Slot>
          </FormInner>
        </FormBody>

        <FormFooter>
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
        </FormFooter>
      </FormPanel>
    </Page>
  );
}

const Page = styled.div`
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1fr 1fr;
  background: var(--afs-bg);

  @media (max-width: 880px) {
    grid-template-columns: 1fr;
  }
`;

const BrandPanel = styled.aside`
  position: relative;
  overflow: hidden;
  display: flex;
  color: var(--afs-ink);
  background: var(--afs-bg-1);
  border-right: 1px solid var(--afs-line);

  &::before {
    content: "";
    position: absolute;
    inset: 0;
    background-image:
      linear-gradient(color-mix(in srgb, var(--afs-ink) 7%, transparent) 1px, transparent 1px),
      linear-gradient(90deg, color-mix(in srgb, var(--afs-ink) 7%, transparent) 1px, transparent 1px);
    background-size: 42px 42px;
    opacity: 0.36;
    pointer-events: none;
  }

  @media (max-width: 880px) {
    display: none;
  }
`;

const BrandInner = styled.div`
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 32px;
  padding: 48px 56px;
  width: 100%;
  max-width: 680px;
  margin-left: auto;

  @media (max-width: 1080px) {
    padding: 40px 40px;
  }

  @media (max-width: 880px) {
    max-width: none;
    margin: 0;
    padding: 32px 24px;
  }
`;

const BrandLogo = styled.div`
  display: inline-flex;
  align-items: center;

  svg {
    height: 48px !important;
    width: 155px !important;
  }
`;

const BrandBody = styled.div`
  margin-top: auto;
  margin-bottom: auto;
  display: grid;
`;

const Headline = styled.h1`
  margin: 0;
  font-size: clamp(32px, 3.8vw, 48px);
  line-height: 1.04;
  letter-spacing: 0;
  font-weight: 700;
  color: var(--afs-ink);

  s {
    font-style: normal;
    color: var(--afs-ink-dim);
    text-decoration-color: var(--afs-redis-red);
    text-decoration-thickness: 0.12em;
  }
`;

const FormPanel = styled.section`
  position: relative;
  display: flex;
  flex-direction: column;
  background: var(--afs-bg);

  [data-theme="light"] & {
    background: #ffffff;
  }

  @media (max-width: 880px) {
    min-height: 100vh;
  }
`;

const FormTopBar = styled.header`
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 24px 32px 0;
`;

const MobileLogoLink = styled(Link)`
  display: none;
  color: inherit;

  svg {
    height: 38px !important;
    width: 123px !important;
  }

  @media (max-width: 880px) {
    display: inline-flex;
  }
`;

const AuthThemeToggle = styled(ThemeModeToggle)`
  margin-left: auto;
`;

const FormBody = styled.div`
  flex: 1;
  display: grid;
  place-items: center;
  padding: 24px 32px 48px;
`;

const FormInner = styled.div`
  width: 100%;
  max-width: 440px;
  display: grid;
  gap: 28px;
`;

const Heading = styled.div`
  display: grid;
  gap: 10px;
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
`;

const FormFooter = styled.footer`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 32px;
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
