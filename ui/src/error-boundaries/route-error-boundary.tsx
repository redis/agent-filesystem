import { useRouter } from "@tanstack/react-router";
import type { ErrorComponentProps } from "@tanstack/react-router";
import { ErrorFallback } from "./error-fallback";
import { getErrorMessage } from "./get-error-message";

export function RouteErrorBoundary({
  error,
  reset,
  fullPage,
}: ErrorComponentProps & { fullPage?: boolean }) {
  const router = useRouter();

  return (
    <ErrorFallback
      actionLabel="Try again"
      fullPage={fullPage}
      message={getErrorMessage(error)}
      onAction={() => {
        reset();
        void router.invalidate();
      }}
      title="Something went wrong"
    />
  );
}
