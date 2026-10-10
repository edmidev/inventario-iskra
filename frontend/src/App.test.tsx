import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { App } from "./App";

describe("App shell", () => {
  it("renders the bottom nav and navigates between placeholder routes", async () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );

    const nav = screen.getByRole("navigation", { name: /principal/i });
    for (const label of ["Inicio", "Materiales", "Escanear", "Más"]) {
      expect(nav).toHaveTextContent(label);
    }
    expect(screen.getByRole("heading", { name: "Inicio" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("link", { name: "Materiales" }));
    expect(screen.getByRole("heading", { name: "Materiales" })).toBeInTheDocument();
  });
});
