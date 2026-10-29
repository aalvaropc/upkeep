import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Forecast } from "../types";
import { ForecastView } from "./DetailPanel";

const forecast: Forecast = {
  module: "github.com/jonboulle/clockwork",
  from: "v0.4.0",
  to: "v0.5.0",
  summary: "",
  breaking: [{ symbol: "clockwork.NewFakeClock", description: "removed in v0.5.0" }],
  dropped: 0,
  excerpt: "",
  excerpt_source: "",
  files: [],
  sources: ["https://github.com/jonboulle/clockwork/releases/tag/v0.5.0"],
};

function lead(f: Forecast): string {
  const { container } = render(<ForecastView forecast={f} confirmed={[]} missed={[]} />);
  return container.querySelector("p")?.textContent ?? "";
}

afterEach(cleanup);

describe("ForecastView", () => {
  it("keeps the text of a forecast read from release notes", () => {
    expect(lead(forecast)).toBe(
      "Super read 1 release note for github.com/jonboulle/clockwork v0.4.0 to v0.5.0 before any fix, and predicted what would break. The compiler then said whether it did.",
    );
    expect(lead({ ...forecast, sources: [] })).toBe(
      "Super read the release notes and the migration guide for github.com/jonboulle/clockwork v0.4.0 to v0.5.0 before any fix, and predicted what would break. The compiler then said whether it did.",
    );
  });

  it("says when the forecast comes from the module's exported API", () => {
    const text = lead({ ...forecast, source: "api", sources: [] });
    expect(text).toBe(
      "No release notes or web pages covered github.com/jonboulle/clockwork v0.4.0 to v0.5.0, so this forecast comes from the module's exported API: before any fix, Upkeep compared both versions for the identifiers the project uses. The compiler then said whether it broke.",
    );
  });

  it("says when no identifier the project uses changed in the API", () => {
    const { container } = render(
      <ForecastView
        forecast={{ ...forecast, source: "api", sources: [], breaking: [] }}
        confirmed={[]}
        missed={[]}
      />,
    );
    expect(container.textContent).toContain(
      "None of the identifiers the project uses changed in the module's exported API.",
    );
  });

  it("marks a prediction the fix changed and a build that passed", () => {
    const changed = render(
      <ForecastView
        forecast={forecast}
        confirmed={[]}
        missed={[]}
        fixed={["clockwork.NewFakeClock"]}
      />,
    );
    expect(changed.container.textContent).toContain("changed by the fix");
    cleanup();
    const passed = render(
      <ForecastView
        forecast={forecast}
        confirmed={[]}
        missed={["clockwork.NewFakeClock"]}
        buildPassed
      />,
    );
    expect(passed.container.textContent).toContain("the build passed");
    expect(passed.container.textContent).not.toContain("not seen in the build");
  });
});
