import { describe, expect, it } from "vitest";
import { permissionsFor } from "./session";
import { user } from "../../test/fixtures";

describe("effective permissions", () => {
  it("never infers grants from an administrator role name", () => {
    expect(permissionsFor({ ...user("admin"), permissions: [] })).toMatchObject({ canWriteItems: false, canOperate: false, isAdmin: false });
  });
  it("honors server grants for any role", () => {
    expect(permissionsFor({ ...user("worker"), permissions: ["items:write", "users:manage"] })).toMatchObject({ canWriteItems: true, canOperate: false, canManageUsers: true, canManageRoles: false });
  });
  it("grants nothing without a session", () => {
    expect(permissionsFor(null)).toMatchObject({ canWriteItems: false, canOperate: false, isAdmin: false });
  });
});
