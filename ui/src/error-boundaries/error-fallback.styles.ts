import { Card } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";

export const CenteredLayout = styled.div<{ $fullPage?: boolean }>`
  align-items: center;
  display: flex;
  flex: 1;
  justify-content: center;
  min-height: ${({ $fullPage }) => ($fullPage ? "100vh" : "100%")};
  padding: 24px;
`;

export const FallbackCard = styled(Card)`
  max-width: 480px;
  width: 100%;
`;

export const MessageStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
`;
