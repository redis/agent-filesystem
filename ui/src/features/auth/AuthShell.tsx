import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import styled, { createGlobalStyle } from "styled-components";
import { RedisLogoDarkFullIcon } from "@redis-ui/icons/multicolor";
import { ThemeModeToggle } from "../../components/theme-mode-toggle";
import { searchBenchmark } from "../../foundation/performance-data";

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
            <Pill>Agent Filesystem · Cloud</Pill>
            <Headline>
              a filesystem for <s>humans</s> agents
            </Headline>
            <TerminalWindow aria-label="CLI quickstart example">
              <TerminalHeader>
                <TerminalLights aria-hidden="true">
                  <span />
                  <span />
                  <span />
                </TerminalLights>
                <TerminalTitle>terminal</TerminalTitle>
              </TerminalHeader>
              <TerminalBody>
                <TerminalComment>// Create a new workspace and attach it</TerminalComment>
                <TerminalLine>&gt; afs ws create myworkspace</TerminalLine>
                <TerminalLine>&gt; afs ws attach myworkspace ~/myworkspace</TerminalLine>
                <TerminalGap />
                <TerminalComment>// Create a checkpoint</TerminalComment>
                <TerminalLine>&gt; afs cp create myworkspace initial</TerminalLine>
              </TerminalBody>
            </TerminalWindow>
            <BenchmarkPanel>
              <BenchmarkHeader>
                <BenchmarkEyebrow>Indexed search</BenchmarkEyebrow>
                <BenchmarkTitle>{searchBenchmark.title}</BenchmarkTitle>
                <BenchmarkContext>{searchBenchmark.corpus}</BenchmarkContext>
              </BenchmarkHeader>
              <BenchmarkGrid>
                {searchBenchmark.metrics.map((metric) => (
                  <BenchmarkItem key={metric.name}>
                    <BenchmarkName>{metric.name}</BenchmarkName>
                    <BenchmarkValue>{metric.afs}</BenchmarkValue>
                    <BenchmarkMeta>{metric.summary}</BenchmarkMeta>
                  </BenchmarkItem>
                ))}
              </BenchmarkGrid>
            </BenchmarkPanel>
          </BrandBody>

          <BrandFooter>Trusted by teams shipping production agents on Redis.</BrandFooter>
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
    border-right: none;
    border-bottom: 1px solid var(--afs-line);
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
  gap: 22px;
`;

const Pill = styled.div`
  width: fit-content;
  padding: 6px 12px;
  border-radius: 999px;
  background: var(--afs-accent-soft);
  border: 1px solid var(--afs-line-strong);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--afs-ink);
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

const TerminalWindow = styled.div`
  overflow: hidden;
  border: 1px solid var(--afs-line-strong);
  border-radius: 16px;
  background: var(--afs-bg);
  box-shadow: var(--afs-shadow-2);
`;

const TerminalHeader = styled.div`
  height: 38px;
  padding: 0 14px;
  display: flex;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid var(--afs-line);
  background: var(--afs-bg-2);
`;

const TerminalLights = styled.span`
  display: inline-flex;
  gap: 7px;

  span {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: #dc382c;
  }

  span:nth-child(2) {
    background: #f5a524;
  }

  span:nth-child(3) {
    background: #4ac97a;
  }
`;

const TerminalTitle = styled.span`
  color: var(--afs-muted);
  font-family: "Redis Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
`;

const TerminalBody = styled.div`
  padding: 18px 20px 20px;
  display: grid;
  gap: 7px;
  font-family: "Redis Mono", "SFMono-Regular", Consolas, monospace;
  font-size: clamp(13px, 1.15vw, 15px);
  line-height: 1.5;
`;

const TerminalLine = styled.div`
  color: var(--afs-ink);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
`;

const TerminalComment = styled(TerminalLine)`
  color: var(--afs-muted);
`;

const TerminalGap = styled.div`
  height: 8px;
`;

const BenchmarkPanel = styled.section`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 18px;
  background: var(--afs-panel);
  display: grid;
  gap: 16px;
`;

const BenchmarkHeader = styled.div`
  display: grid;
  gap: 4px;
`;

const BenchmarkEyebrow = styled.span`
  color: var(--afs-accent);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const BenchmarkTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 17px;
  line-height: 1.25;
`;

const BenchmarkContext = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.45;
`;

const BenchmarkGrid = styled.div`
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 1180px) {
    grid-template-columns: 1fr;
  }
`;

const BenchmarkItem = styled.div`
  min-width: 0;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  padding: 14px;
  background: var(--afs-bg-soft);
  display: grid;
  gap: 6px;
`;

const BenchmarkName = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
`;

const BenchmarkValue = styled.span`
  color: var(--afs-ink);
  font-family: "Redis Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 24px;
  line-height: 1;
`;

const BenchmarkMeta = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
`;

const BrandFooter = styled.p`
  margin: 0;
  font-size: 13px;
  color: var(--afs-muted);
`;

const FormPanel = styled.section`
  position: relative;
  display: flex;
  flex-direction: column;
  background: var(--afs-bg);

  [data-theme="light"] & {
    background: #ffffff;
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
