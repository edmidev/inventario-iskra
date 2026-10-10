import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OfflineBanner } from "./OfflineBanner";

function setOnline(value: boolean) {
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(value);
  act(() => {
    window.dispatchEvent(new Event(value ? "online" : "offline"));
  });
}

afterEach(() => vi.restoreAllMocks());

describe("OfflineBanner", () => {
  it("is hidden while online", () => {
    render(<OfflineBanner />);
    expect(screen.queryByText(/Sin conexión/)).not.toBeInTheDocument();
  });

  it("appears when the connection drops and disappears when it returns", () => {
    render(<OfflineBanner />);
    setOnline(false);
    expect(screen.getByText(/Sin conexión/)).toBeInTheDocument();
    setOnline(true);
    expect(screen.queryByText(/Sin conexión/)).not.toBeInTheDocument();
  });

  it("starts visible when the page loads offline", () => {
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    render(<OfflineBanner />);
    expect(screen.getByRole("status")).toHaveTextContent("Sin conexión");
  });
});
