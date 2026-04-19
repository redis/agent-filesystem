import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import styled, { createGlobalStyle, keyframes } from "styled-components";
import { RedisLogoDarkFullIcon } from "@redis-ui/icons/multicolor";
import { useColorMode } from "../../foundation/theme-context";

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
  const { colorMode, toggleColorMode } = useColorMode();

  return (
    <Page>
      <AuthRootFontSize />
      <BrandPanel>
        <BrandOrbA />
        <BrandOrbB />
        <BrandInner>
          <BrandLogo>
            <RedisLogoDarkFullIcon />
          </BrandLogo>

          <BrandBody>
            <Pill>Agent Filesystem · Cloud</Pill>
            <Headline>
              The filesystem <em>built</em> for autonomous agents.
            </Headline>
            <Lede>
              Give every agent a durable workspace, a shared memory, and a clean trail of
              everything it touched — powered by Redis.
            </Lede>
            <FeatureList>
              <Feature>
                <FeatureDot />
                <FeatureText>
                  <strong>Durable workspaces</strong>
                  <span>Files, folders, and checkpoints that survive every run.</span>
                </FeatureText>
              </Feature>
              <Feature>
                <FeatureDot />
                <FeatureText>
                  <strong>Shared memory for agents</strong>
                  <span>One context store, real-time sync, no stale snapshots.</span>
                </FeatureText>
              </Feature>
              <Feature>
                <FeatureDot />
                <FeatureText>
                  <strong>Auditable by default</strong>
                  <span>Every read, write, and revision — captured automatically.</span>
                </FeatureText>
              </Feature>
            </FeatureList>
          </BrandBody>

          <BrandFooter>Trusted by teams shipping production agents on Redis.</BrandFooter>
        </BrandInner>
      </BrandPanel>

      <FormPanel>
        <FormTopBar>
          <MobileLogoLink to="/">
            <RedisLogoDarkFullIcon />
          </MobileLogoLink>
          <ThemeToggle
            type="button"
            onClick={toggleColorMode}
            aria-label="Toggle theme"
            title="Toggle theme"
          >
            {colorMode === "dark" ? "☀" : "☾"}
          </ThemeToggle>
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

const float = keyframes`
  0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
  50% { transform: translate3d(10px, -16px, 0) scale(1.04); }
`;

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
  color: #f6f1ec;
  background:
    radial-gradient(120% 80% at 10% 0%, rgba(220, 56, 44, 0.32) 0%, rgba(220, 56, 44, 0) 55%),
    radial-gradient(80% 60% at 100% 100%, rgba(255, 138, 92, 0.2) 0%, rgba(255, 138, 92, 0) 60%),
    linear-gradient(160deg, #0d2330 0%, #091a23 48%, #050d13 100%);

  @media (max-width: 880px) {
    display: none;
  }
`;

const BrandOrbA = styled.div`
  position: absolute;
  width: 520px;
  height: 520px;
  top: -180px;
  right: -160px;
  background: radial-gradient(circle, rgba(220, 56, 44, 0.3), rgba(220, 56, 44, 0) 70%);
  filter: blur(4px);
  animation: ${float} 12s ease-in-out infinite;
  pointer-events: none;
`;

const BrandOrbB = styled.div`
  position: absolute;
  width: 420px;
  height: 420px;
  bottom: -140px;
  left: -120px;
  background: radial-gradient(circle, rgba(255, 138, 92, 0.18), rgba(255, 138, 92, 0) 70%);
  filter: blur(2px);
  animation: ${float} 16s ease-in-out infinite reverse;
  pointer-events: none;
`;

const BrandInner = styled.div`
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 40px;
  padding: 48px 56px;
  width: 100%;
  max-width: 620px;
  margin-left: auto;

  @media (max-width: 1080px) {
    padding: 40px 40px;
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
  gap: 24px;
`;

const Pill = styled.div`
  width: fit-content;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.14);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #f6f1ec;
`;

const Headline = styled.h1`
  margin: 0;
  font-size: clamp(32px, 3.8vw, 48px);
  line-height: 1.04;
  letter-spacing: -0.03em;
  font-weight: 700;
  color: #ffffff;

  em {
    font-style: normal;
    background: linear-gradient(120deg, #ff6a4d 0%, #dc382c 50%, #ff9170 100%);
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
  }
`;

const Lede = styled.p`
  margin: 0;
  font-size: 16px;
  line-height: 1.55;
  color: rgba(246, 241, 236, 0.76);
  max-width: 48ch;
`;

const FeatureList = styled.ul`
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 16px;
`;

const Feature = styled.li`
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 14px;
  align-items: start;
`;

const FeatureDot = styled.span`
  margin-top: 6px;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: linear-gradient(135deg, #ff6a4d, #dc382c);
  box-shadow: 0 0 0 4px rgba(220, 56, 44, 0.14);
`;

const FeatureText = styled.div`
  display: grid;
  gap: 2px;
  strong {
    color: #ffffff;
    font-weight: 600;
    font-size: 15px;
  }
  span {
    color: rgba(246, 241, 236, 0.62);
    font-size: 14px;
    line-height: 1.5;
  }
`;

const BrandFooter = styled.p`
  margin: 0;
  font-size: 13px;
  color: rgba(246, 241, 236, 0.48);
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

const ThemeToggle = styled.button`
  margin-left: auto;
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

  &:hover {
    background: var(--afs-panel-strong);
    border-color: var(--afs-line-strong);
  }
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
