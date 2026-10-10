import { render } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { App } from "../App";
import { AppProviders, createQueryClient } from "../providers";
import type { Role } from "../lib/api/schemas";
import { categories, locations, session } from "./fixtures";
import { api } from "./api";
import { server } from "./server";

export { api, errorBody } from "./api";

/** Handlers for an authenticated session plus the catalogs every screen loads. */
export function useAuthenticatedApi(role: Role = "operator") {
  server.use(
    http.post(api("/auth/refresh"), () => HttpResponse.json(session(role))),
    http.get(api("/categories"), () => HttpResponse.json({ data: categories })),
    http.get(api("/locations"), () => HttpResponse.json({ data: locations })),
    // No open failures unless a test says otherwise (operators' item detail asks for it).
    http.get(api("/maintenance"), () => HttpResponse.json({ data: [], total: 0, limit: 1, offset: 0 })),
  );
}

export function renderApp(route = "/") {
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: 0 } });
  const result = render(
    <AppProviders client={client}>
      <MemoryRouter initialEntries={[route]}>
        <App />
      </MemoryRouter>
    </AppProviders>,
  );
  return { ...result, client };
}
