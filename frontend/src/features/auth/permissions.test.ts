import { describe, expect, it } from "vitest";
import { permissionsFor } from "./session";

describe("permissionsFor", () => {
  it.each([
    ["viewer", false, false],
    ["operator", true, false],
    ["admin", true, true],
  ] as const)("%s: canWriteItems=%s isAdmin=%s", (role, canWriteItems, isAdmin) => {
    expect(permissionsFor(role)).toMatchObject({ canWriteItems, isAdmin });
  });

  it("grants nothing without a session", () => {
    expect(permissionsFor(null)).toMatchObject({ canWriteItems: false, isAdmin: false });
  });
});
