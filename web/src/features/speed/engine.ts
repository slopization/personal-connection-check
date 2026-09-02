export type Clock = { now(): number };
export type Phase = { bytes: number; samples: number[] };
export type Transport = {
  now: () => number;
  download: (streams: number, signal: AbortSignal) => Promise<Phase>;
  upload: (
    streams: number,
    signal: AbortSignal,
  ) => Promise<{ ackBytes: number; samples: number[] }>;
};
export type SpeedResult = {
  downloadMbps: number;
  uploadMbps: number;
  durationMs: number;
  streams: number;
};
export const median = (v: number[]) => {
  const x = [...v].sort((a, b) => a - b);
  return x.length ? x[Math.floor(x.length / 2)] : 0;
};
export const stable = (w: number[], threshold = 0.05) =>
  w.length >= 2 &&
  Math.abs(w[w.length - 1] - w[w.length - 2]) / Math.max(1, w[w.length - 2]) <=
    threshold;
const mbps = (bytes: number, samples: number[]) =>
  (median(samples.slice(1)) * 8) / 1_000_000 || (bytes * 8) / 5_000_000;
export async function runAdaptive(
  t: Transport,
  external?: AbortSignal,
): Promise<SpeedResult> {
  const ctl = new AbortController();
  external?.addEventListener("abort", () => ctl.abort(), { once: true });
  const start = t.now();
  let dl: Phase = { bytes: 0, samples: [] },
    up: { ackBytes: number; samples: number[] } = { ackBytes: 0, samples: [] },
    streams = 1,
    previous = 0;
  for (const n of [1, 2, 4, 8]) {
    if (ctl.signal.aborted) throw new DOMException("cancelled", "AbortError");
    streams = n;
    dl = await t.download(n, ctl.signal);
    up = await t.upload(n, ctl.signal);
    const elapsed = t.now() - start;
    const windows = dl.samples.filter((_, i) => i > 0);
    const rate = mbps(dl.bytes, dl.samples);
    const marginal = n === 1 || rate >= previous * 1.05;
    previous = rate;
    if (elapsed >= 5000 && stable(windows) && !marginal) break;
    if (elapsed >= 15000) break;
  }
  return {
    downloadMbps: mbps(dl.bytes, dl.samples),
    uploadMbps: mbps(up.ackBytes, up.samples),
    durationMs: Math.min(15000, t.now() - start),
    streams,
  };
}
export async function adaptive(
  run: (streams: number, signal: AbortSignal) => Promise<number[]>,
  clock: Clock,
  signal: AbortSignal,
) {
  const started = clock.now();
  let last: number[] = [];
  for (const n of [1, 2, 4, 8]) {
    last = await run(n, signal);
    if (clock.now() - started >= 5000 && stable(last)) return median(last);
    if (clock.now() - started >= 15000) break;
  }
  return median(last);
}
