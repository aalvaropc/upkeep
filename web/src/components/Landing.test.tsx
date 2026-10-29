import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Landing } from "./Landing";

describe("Landing", () => {
  const fetch = vi.fn();

  beforeEach(() => {
    fetch.mockReset();
    vi.stubGlobal("fetch", fetch);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("has one top heading and sends the primary action to the demo", () => {
    render(<Landing />);
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    const primary = screen.getByRole("link", { name: "Watch it work the queue" });
    expect(primary.getAttribute("href")).toBe("/demo");
    expect(
      screen.getByRole("link", { name: "See the queue worked live" }).getAttribute("href"),
    ).toBe("/demo");
    expect(
      screen.getByRole("link", { name: "Install the GitHub Action" }).getAttribute("href"),
    ).toBe("https://github.com/aalvaropc/upkeep/blob/main/docs/action.md");
  });

  it("gives every section a heading", () => {
    const { container } = render(<Landing />);
    const sections = container.querySelectorAll("section");
    expect(sections.length).toBeGreaterThanOrEqual(9);
    sections.forEach((section) => {
      expect(section.querySelector("h1, h2")).not.toBeNull();
      const label = section.getAttribute("aria-labelledby");
      expect(label && section.querySelector(`#${label}`)).toBeTruthy();
    });
  });

  it("says it is open source and marks the savings as an example", () => {
    render(<Landing />);
    const heading = screen.getByRole("heading", { level: 2, name: "Open source and cheap to run" });
    const section = heading.closest("section");
    if (!section) {
      throw new Error("the cost section is missing");
    }
    const text = section.textContent ?? "";
    expect(text).toContain("MIT licensed");
    expect(text).toContain("An assumption");
    expect(text).not.toMatch(/Enterprise|Planned/);
  });

  it("shows the product with described screenshots", () => {
    const { container } = render(<Landing />);
    const images = Array.from(container.querySelectorAll("img"));
    expect(images.map((img) => img.getAttribute("src"))).toEqual([
      "/landing/dashboard-trace.png",
      "/landing/dashboard-plan.png",
      "/landing/dashboard-detail.png",
      "/landing/dashboard-learned.png",
    ]);
    images.forEach((img) => {
      expect((img.getAttribute("alt") ?? "").length).toBeGreaterThan(40);
      expect(img.getAttribute("width")).toBeTruthy();
      expect(img.getAttribute("height")).toBeTruthy();
    });
  });

  it("cites its sources and never calls the network", () => {
    render(<Landing />);
    expect(
      screen
        .getByRole("link", { name: "Brindescu et al., Empirical Software Engineering, 2019" })
        .getAttribute("href"),
    ).toBe("https://doi.org/10.1007/s10664-019-09735-4");
    expect(fetch).not.toHaveBeenCalled();
  });
});
