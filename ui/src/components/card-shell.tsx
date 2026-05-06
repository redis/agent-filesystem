import styled, { css } from "styled-components";

export const cardSurface = css`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel);
  box-shadow: 0 10px 24px rgba(8, 6, 13, 0.08);

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-1);
    box-shadow: var(--afs-shadow-2);
  }
`;

export const SurfaceCard = styled.div`
  ${cardSurface}
`;
