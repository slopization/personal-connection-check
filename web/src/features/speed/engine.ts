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

export const DIRECTION_LIMIT_MS = 15_000;
const TARGET_MS = 5_000;
const RAMP = [1, 2, 4, 8];

export const median = (v: number[]) => {
  const x = [...v].sort((a, b) => a - b);
  return x.length ? x[Math.floor(x.length / 2)] : 0;
};
export const stable = (w: number[], threshold = 0.05) =>
  w.length >= 2 &&
  Math.abs(w[w.length - 1] - w[w.length - 2]) / Math.max(1, w[w.length - 2]) <=
    threshold;

// Transport samples are already decimal megabits per second.
const rate = (samples: number[]) => median(samples.slice(1)) || median(samples);

type Measured = { rate: number; streams: number };
type Work = (streams: number, signal: AbortSignal) => Promise<Phase>;
type UploadWork = (
  streams: number,
  signal: AbortSignal,
) => Promise<{ ackBytes: number; samples: number[] }>;

async function measureDirection(
  now: () => number,
  work: Work | UploadWork,
  external: AbortSignal | undefined,
  limitMs: number,
): Promise<Measured> {
  const ctl = new AbortController();
  let expired = false;
  const timer = window.setTimeout(() => {
    expired = true;
    ctl.abort();
  }, limitMs);
  const cancel = () => ctl.abort();
  external?.addEventListener("abort", cancel, { once: true });
  const started = now();
  let previous = 0;
  let result: Measured = { rate: 0, streams: 1 };

  try {
    for (const streams of RAMP) {
      if (external?.aborted) throw new DOMException("cancelled", "AbortError");
      if (ctl.signal.aborted) break;
      try {
        const phase = await work(streams, ctl.signal);
        result = { rate: rate(phase.samples), streams };
        const elapsed = now() - started;
        const marginal = previous > 0 && result.rate < previous * 1.05;
        previous = result.rate;
        if (elapsed >= TARGET_MS && (stable(phase.samples) || marginal)) break;
        if (elapsed >= limitMs) break;
      } catch (error) {
        if (external?.aborted) throw error;
        if (!expired) throw error;
        break;
      }
    }
    return result;
  } finally {
    window.clearTimeout(timer);
    external?.removeEventListener("abort", cancel);
  }
}

export async function runAdaptive(
  t: Transport,
  external?: AbortSignal,
  directionLimitMs = DIRECTION_LIMIT_MS,
): Promise<SpeedResult> {
  const started = t.now();
  const download = await measureDirection(
    t.now,
    t.download,
    external,
    directionLimitMs,
  );
  if (external?.aborted) throw new DOMException("cancelled", "AbortError");
  const upload = await measureDirection(
    t.now,
    t.upload,
    external,
    directionLimitMs,
  );
  if (external?.aborted) throw new DOMException("cancelled", "AbortError");
  return {
    downloadMbps: download.rate,
    uploadMbps: upload.rate,
    durationMs: t.now() - started,
    streams: Math.max(download.streams, upload.streams),
  };
}

export async function adaptive(
  run: (streams: number, signal: AbortSignal) => Promise<number[]>,
  clock: Clock,
  signal: AbortSignal,
) {
  const result = await measureDirection(
    () => clock.now(),
    async (streams, directionSignal) => ({
      bytes: 0,
      samples: await run(streams, directionSignal),
    }),
    signal,
    DIRECTION_LIMIT_MS,
  );
  return result.rate;
}
