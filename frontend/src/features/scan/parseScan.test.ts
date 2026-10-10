import { describe, expect, it } from "vitest";
import { extractItemCode, labelUrl } from "./parseScan";

describe("extractItemCode", () => {
  it("accepts a bare code and normalizes case and whitespace", () => {
    expect(extractItemCode("ISK-HER-001")).toBe("ISK-HER-001");
    expect(extractItemCode("  isk-her-001 \n")).toBe("ISK-HER-001");
  });

  it("extracts the code from label URLs", () => {
    expect(extractItemCode("https://inv.example.com/materiales/codigo/ISK-HER-001")).toBe("ISK-HER-001");
    expect(extractItemCode("https://inv.example.com/materiales/by-code/ISK-HER-001?x=1")).toBe("ISK-HER-001");
    expect(extractItemCode("/materiales/codigo/isk%2Dher-002")).toBe("ISK-HER-002");
    expect(extractItemCode("https://api.example.com/api/v1/items/by-code/ISK-CON-010")).toBe("ISK-CON-010");
  });

  it("rejects empty or unusable payloads", () => {
    expect(extractItemCode("")).toBeNull();
    expect(extractItemCode("   ")).toBeNull();
    expect(extractItemCode("hello world")).toBeNull();
    expect(extractItemCode("https://example.com/other/page")).toBeNull();
    expect(extractItemCode("/materiales/codigo/%E0%A4%A")).toBeNull();
  });
});

describe("labelUrl", () => {
  it("builds the resolver URL without double slashes", () => {
    expect(labelUrl("https://inv.example.com/", "ISK-HER-001")).toBe("https://inv.example.com/materiales/codigo/ISK-HER-001");
  });
});
