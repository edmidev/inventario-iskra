import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { QuantityStepper } from "./QuantityStepper";

describe("quantity stepper", () => {
 it("preserves fractions when incrementing", async () => {
  const change=vi.fn<(value: string) => void>(); render(<QuantityStepper decimal value="0.125" onChange={change} />);
  await userEvent.click(screen.getByRole("button",{name:"Aumentar cantidad"}));
  expect(change).toHaveBeenCalledWith("1.125");
  expect(screen.getByRole("spinbutton")).toHaveAttribute("step","0.001");
 });
 it("keeps the default whole-unit behavior", async () => {
  const change=vi.fn<(value: string) => void>(); render(<QuantityStepper value="2" onChange={change} />);
  await userEvent.click(screen.getByRole("button",{name:"Disminuir cantidad"}));
  expect(change).toHaveBeenCalledWith("1");
 });
});
