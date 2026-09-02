import { afterEach, describe, expect, it, vi } from "vitest";
import { runAdaptive } from "./engine";

const phase = (rate: number) => ({
  bytes: 125_000,
  samples: [rate, rate, rate],
});

describe("adaptive speed engine", () => {
  afterEach(() => vi.useRealTimers());

  it("measures the complete download ramp before the upload ramp and preserves Mbps samples", async () => {
    const calls: string[] = [];
    let now = 0;
    const transport = {
      now: () => now,
      download: async (streams: number) => {
        calls.push(`download:${streams}`);
        now += 1_250;
        return phase(100);
      },
      upload: async (streams: number) => {
        calls.push(`upload:${streams}`);
        now += 1_250;
        return { ackBytes: 125_000, samples: [80, 80, 80] };
      },
    };

    const result = await runAdaptive(transport);

    expect(calls).toEqual([
      "download:1",
      "download:2",
      "download:4",
      "download:8",
      "upload:1",
      "upload:2",
      "upload:4",
      "upload:8",
    ]);
    expect(result.downloadMbps).toBe(100);
    expect(result.uploadMbps).toBe(80);
    expect(result.durationMs).toBe(10_000);
  });

  it("gives download and upload independent bounded deadlines", async () => {
    const aborted: string[] = [];
    const transport = {
      now: () => 0,
      download: (_streams: number, signal: AbortSignal) =>
        new Promise<{ bytes: number; samples: number[] }>((resolve) => {
          signal.addEventListener(
            "abort",
            () => {
              aborted.push("download");
              resolve(phase(10));
            },
            { once: true },
          );
        }),
      upload: (_streams: number, signal: AbortSignal) =>
        new Promise<{ ackBytes: number; samples: number[] }>((resolve) => {
          signal.addEventListener(
            "abort",
            () => {
              aborted.push("upload");
              resolve({ ackBytes: 1, samples: [20] });
            },
            { once: true },
          );
        }),
    };

    await runAdaptive(transport, undefined, 10);
    expect(aborted).toEqual(["download", "upload"]);
  });

  it("finishes both direction deadlines when browser work ignores abort", async () => {
    vi.useFakeTimers();
    let settled = false;
    const never = () => new Promise<never>(() => undefined);
    const run = runAdaptive(
      {
        now: () => Date.now(),
        download: never,
        upload: never,
      },
      undefined,
      100,
    ).then((result) => {
      settled = true;
      return result;
    });

    await vi.advanceTimersByTimeAsync(200);

    expect(settled).toBe(true);
    await expect(run).resolves.toMatchObject({
      downloadMbps: 0,
      uploadMbps: 0,
    });
  });

  it("cancels an active direction promptly when the caller cancels", async () => {
    const ctl = new AbortController();
    const transport = {
      now: () => 0,
      download: (_streams: number, signal: AbortSignal) =>
        new Promise<{ bytes: number; samples: number[] }>((_resolve, reject) =>
          signal.addEventListener(
            "abort",
            () => reject(new DOMException("cancelled", "AbortError")),
            { once: true },
          ),
        ),
      upload: async () => ({ ackBytes: 0, samples: [] }),
    };
    const run = runAdaptive(transport, ctl.signal, 100);
    ctl.abort();
    await expect(run).rejects.toMatchObject({ name: "AbortError" });
  });
});
