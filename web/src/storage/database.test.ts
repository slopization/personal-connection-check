import "fake-indexeddb/auto";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  clear,
  latestStability,
  list,
  listStability,
  save,
  saveStability,
} from "./database";

describe("history database", () => {
  beforeEach(async () => {
    await clear();
    Object.defineProperty(navigator, "storage", {
      configurable: true,
      value: { estimate: vi.fn(async () => ({ quota: 5_000 })) },
    });
  });

  it("evicts the oldest record across both stores before a write exceeds budget", async () => {
    await save({ at: 1, download: 1, upload: 1, ping: 1, isp: "x".repeat(80) });
    await saveStability({
      at: 2,
      samples: [{ at: 2, rtt: 2 }],
      pauses: [],
    });
    await save({ at: 3, download: 3, upload: 3, ping: 3, isp: "x".repeat(80) });

    expect((await list()).map((row) => row.at)).toEqual([3]);
    expect((await listStability()).map((row) => row.at)).toEqual([2]);
  });

  it("returns saved stability sessions newest first and exposes the latest session", async () => {
    await saveStability({ at: 10, samples: [{ at: 10, rtt: 4 }], pauses: [9] });
    await saveStability({
      at: 20,
      samples: [{ at: 20, rtt: 5 }],
      pauses: [19],
    });

    expect((await listStability()).map((session) => session.at)).toEqual([
      20, 10,
    ]);
    await expect(latestStability()).resolves.toMatchObject({
      at: 20,
      samples: [{ at: 20, rtt: 5 }],
      pauses: [19],
    });
  });
});
