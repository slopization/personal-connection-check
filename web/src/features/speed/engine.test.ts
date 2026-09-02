import { describe, expect, it } from "vitest";
import { runAdaptive } from "./engine";
describe("adaptive speed engine", () => {
  it("ramps streams and uses server acknowledged upload bytes", async () => {
    const calls: number[] = [];
    const result = await runAdaptive({
      now: (() => {
        let n = 0;
        return () => (n += 3000);
      })(),
      download: async (streams) => {
        calls.push(streams);
        return { bytes: streams * 125000, samples: [100, 100, 101, 100, 100] };
      },
      upload: async (streams) => ({
        ackBytes: streams * 125000,
        samples: [90, 90, 91, 90, 90],
      }),
    });
    expect(calls).toEqual([1, 2]);
    expect(result.downloadMbps).toBeGreaterThan(0);
    expect(result.uploadMbps).toBeGreaterThan(0);
  });
});
