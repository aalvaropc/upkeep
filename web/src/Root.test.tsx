import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isDemoPath, Root } from "./Root";

vi.mock("./App", () => ({
  App: () => <p>the dashboard</p>,
}));

describe("Root", () => {
  afterEach(() => {
    cleanup();
  });

  it("shows the landing page at the root path", () => {
    render(<Root pathname="/" />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain(
      "Clear the pull request queue.",
    );
    expect(screen.queryByText("the dashboard")).toBeNull();
  });

  it("shows the dashboard under /demo", () => {
    for (const pathname of ["/demo", "/demo/"]) {
      render(<Root pathname={pathname} />);
      expect(screen.getByText("the dashboard")).toBeTruthy();
      expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
      cleanup();
    }
  });

  it("reads the path from the location by default", () => {
    window.history.pushState({}, "", "/demo");
    render(<Root />);
    expect(screen.getByText("the dashboard")).toBeTruthy();
    window.history.pushState({}, "", "/");
  });

  it("treats only paths under /demo as the dashboard", () => {
    expect(isDemoPath("/demo")).toBe(true);
    expect(isDemoPath("/demo/")).toBe(true);
    expect(isDemoPath("/")).toBe(false);
    expect(isDemoPath("/docs")).toBe(false);
  });
});
