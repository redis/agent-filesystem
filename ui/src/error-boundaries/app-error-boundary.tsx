import { Component } from "react";
import type {
  ErrorInfo,
  PropsWithChildren,
} from "react";
import { ErrorFallback } from "./error-fallback";
import { getErrorMessage } from "./get-error-message";

type AppErrorBoundaryState = {
  error: Error | null;
};

export class AppErrorBoundary extends Component<
  PropsWithChildren,
  AppErrorBoundaryState
> {
  state: AppErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): AppErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Unhandled app error", error, info);
  }

  handleReset = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return (
        <ErrorFallback
          actionLabel="Retry"
          fullPage
          message={getErrorMessage(this.state.error)}
          onAction={this.handleReset}
          title="Application failed to load"
        />
      );
    }

    return this.props.children;
  }
}
