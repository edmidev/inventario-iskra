import { describe, expect, it } from "vitest";
import { CameraError, classifyCameraError } from "./scanner";

describe("classifyCameraError", () => {
  it("maps permission errors", () => {
    expect(classifyCameraError(new DOMException("x", "NotAllowedError"))).toBe("denied");
    expect(classifyCameraError(new Error("Permission denied"))).toBe("denied");
  });
  it("maps missing devices", () => {
    expect(classifyCameraError(new DOMException("x", "NotFoundError"))).toBe("no-camera");
    expect(classifyCameraError("Camera not found")).toBe("failed");
  });
  it("keeps explicit camera errors and defaults to failed", () => {
    expect(classifyCameraError(new CameraError("insecure", "x"))).toBe("insecure");
    expect(classifyCameraError(new Error("boom"))).toBe("failed");
  });
});
