import { describe, expect, it } from "vitest";
import { filteredSamples } from "./StabilityChart";
describe("stability chart filtering", () =>
  it("does not mutate session history", () => {
    const samples = [
      { at: 1, rtt: 3 },
      { at: Date.now(), rtt: 4 },
    ];
    const copy = [...samples];
    expect(filteredSamples(samples, "10m")).toHaveLength(1);
    expect(samples).toEqual(copy);
  }));
