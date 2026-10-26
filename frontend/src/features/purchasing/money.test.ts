import { describe, expect, it } from "vitest";
import { formatMinor, minorToInput, parseMinorUnits } from "./money";

describe("parseMinorUnits", () => {
  it.each([
    ["12", 1200],
    ["12.5", 1250],
    ["12,50", 1250],
    ["0", 0],
    ["0.07", 7],
    [" 3.10 ", 310],
    ["19.99", 1999],
    ["1.15", 115],
  ])("converts %s to %d without float drift", (text, minor) => {
    expect(parseMinorUnits(text)).toBe(minor);
  });

  it.each(["", "abc", "-1", "1.234", "1.", ".5", "1e3", "9".repeat(20)])("rejects %s", (text) => {
    expect(parseMinorUnits(text)).toBeNull();
  });
});

describe("minorToInput", () => {
  it("round-trips with parseMinorUnits", () => {
    for (const minor of [0, 7, 100, 1250, 123456]) {
      expect(parseMinorUnits(minorToInput(minor))).toBe(minor);
    }
    expect(minorToInput(1250)).toBe("12.50");
  });
});

describe("formatMinor", () => {
  it("prefixes the currency symbol", () => {
    expect(formatMinor(123450, "USD")).toBe("US$ 1,234.50");
    expect(formatMinor(5, "VES")).toBe("Bs. 0.05");
  });
});
