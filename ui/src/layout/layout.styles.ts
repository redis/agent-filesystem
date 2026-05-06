import styled from "styled-components";

export const FlexRow = styled.div`
  display: flex;
  height: 100%;
  min-height: 0;
  overflow: hidden;
`;

export const FlexCol = styled(FlexRow)`
  flex-direction: column;
  min-height: auto;
`;

export const FlexColItem = styled(FlexCol)`
  flex: 1;
  height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
`;

export const MainContainer = styled.main`
  display: flex;
  height: 100%;
  min-height: 0;
  min-width: 0;
  position: relative;
  overflow-x: hidden;
  overflow-y: auto;
  flex-direction: column;
  flex: 1;
  background-color: transparent;
`;
