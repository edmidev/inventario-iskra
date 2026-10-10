import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { BrowserRouter } from "react-router-dom";
import { TooltipProvider } from "./components/atoms/Tooltip";
import { ToastProvider } from "./components/molecules/Toast";
import { SessionProvider } from "./features/auth/session";
import { ApiError } from "./lib/api/errors";

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        refetchOnWindowFocus: false,
        // Client errors will not fix themselves; retry only network/5xx failures, twice at most.
        retry: (failures, error) => !(error instanceof ApiError && error.status >= 400 && error.status < 500) && failures < 2,
      },
    },
  });
}

/** Providers shared by the app and the tests (the router is supplied by the caller). */
export function AppProviders({ client, children }: { client: QueryClient; children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>
      <TooltipProvider delayDuration={300}>
        <ToastProvider>
          <SessionProvider>{children}</SessionProvider>
        </ToastProvider>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

export function Root({ client, children }: { client: QueryClient; children: ReactNode }) {
  return (
    <AppProviders client={client}>
      <BrowserRouter>{children}</BrowserRouter>
    </AppProviders>
  );
}
