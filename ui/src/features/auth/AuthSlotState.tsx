import styled from "styled-components";
import { Loader } from "@redis-ui/components";

type Kind = "loading" | "unsupported";

export function AuthSlotState({ kind }: { kind: Kind }) {
  if (kind === "loading") {
    return (
      <SlotCenter aria-busy="true">
        <Loader data-testid="loader--spinner" />
      </SlotCenter>
    );
  }
  return (
    <SlotCenter>
      <UnsupportedText>
        Account-based sign in is not enabled on this deployment. Authentication is managed by your
        organization&apos;s identity provider.
      </UnsupportedText>
    </SlotCenter>
  );
}

const SlotCenter = styled.div`
  min-height: 280px;
  display: grid;
  place-items: center;
  padding: 24px 0;
`;

const UnsupportedText = styled.p`
  margin: 0;
  max-width: 36ch;
  text-align: center;
  color: var(--afs-muted);
  line-height: 1.55;
`;
