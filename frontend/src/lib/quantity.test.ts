import { describe, expect, it } from "vitest";
import { parseQuantity, quantityAfter, lineCostMinor } from "./quantity";

describe("exact three-decimal quantities", () => {
 it("parses fractions and preserves integer inputs", () => {
  expect(parseQuantity("0.125")).toBe(0.125);
  expect(parseQuantity("12")).toBe(12);
  expect(parseQuantity("0.0001")).toBeNull();
  expect(parseQuantity("NaN")).toBeNull();
 });
 it("computes exact human-unit previews and half-up costs", () => {
  expect(quantityAfter(0.1,0.2)).toBe(0.3);
  expect(lineCostMinor("0.125",100)).toBe(13);
  expect(lineCostMinor("2",250)).toBe(500);
 });
});
