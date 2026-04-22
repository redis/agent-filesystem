export function shouldEnableConnectCLIQueries(input: { isLoading: boolean; isAuthenticated: boolean }) {
  return !input.isLoading && input.isAuthenticated;
}
