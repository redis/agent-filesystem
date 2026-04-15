import {
  Banner,
  Typography,
} from "@redis-ui/components";
import {
  CenteredLayout,
  FallbackCard,
  MessageStack,
} from "./error-fallback.styles";

type ErrorFallbackProps = {
  actionLabel: string;
  fullPage?: boolean;
  message: string;
  onAction: () => void;
  title: string;
};

export function ErrorFallback({
  actionLabel,
  fullPage,
  message,
  onAction,
  title,
}: ErrorFallbackProps) {
  return (
    <CenteredLayout
      as={fullPage ? "main" : "div"}
      $fullPage={fullPage}
      aria-live="assertive"
    >
      <FallbackCard>
        <Banner
          actions={{
            primary: {
              label: actionLabel,
              onClick: onAction,
            },
          }}
          layoutVariant="banner"
          message={
            <MessageStack>
              <Typography.Heading component="h1" size="M">
                {title}
              </Typography.Heading>
              <Typography.Body color="secondary" component="p">
                {message}
              </Typography.Body>
            </MessageStack>
          }
          show
          showIcon
          size="M"
          variant="danger"
        />
      </FallbackCard>
    </CenteredLayout>
  );
}
