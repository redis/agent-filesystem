import styled from "styled-components";

export const FlexRow = styled.div`
  display: flex;
  min-height: 100vh;
`;

export const FlexCol = styled(FlexRow)`
  flex-direction: column;
  min-height: auto;
`;

export const FlexColItem = styled(FlexCol)`
  flex: 1;
  height: 100vh;
  min-width: 0;
  overflow: hidden;
`;

export const MainContainer = styled.main`
  display: flex;
  min-width: 0;
  position: relative;
  overflow-x: hidden;
  overflow-y: auto;
  flex-direction: column;
  flex: 1;
  background-color: transparent;
`;
