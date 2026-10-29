import { describe, expect, it } from "vitest";
import { shortVersion } from "./StatusBadge";

describe("shortVersion", () => {
  it("keeps a release as it is", () => {
    expect(shortVersion("v0.1.1")).toBe("v0.1.1");
  });

  it("shortens a pseudo-version to the release and the commit", () => {
    expect(shortVersion("v0.1.1-0.20261029025611-6834ae3c1d2f")).toBe("v0.1.1 · 6834ae3");
  });

  it("marks a dirty build", () => {
    expect(shortVersion("v0.1.1-0.20261029025611-6834ae3c1d2f+dirty")).toBe(
      "v0.1.1 · 6834ae3 dirty",
    );
  });
});
