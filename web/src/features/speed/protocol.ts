import type { Phase, Transport } from "./engine";

type Fetcher = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;
export type LiveSpeedSample = {
  at: number;
  direction: "download" | "upload";
  mbps: number;
};
const CHUNK = 1024 * 1024;
const PHASE_MS = 1250;
const DOWNLOAD_TIMEOUT_MS = PHASE_MS + 500;
const DOWNLOAD_AGGREGATE_TIMEOUT_MS = DOWNLOAD_TIMEOUT_MS + 300;
const MAX_UPLOAD_REQUESTS = 8;
const mbps = (bytes: number, elapsed: number) =>
  elapsed > 0 ? (bytes * 8 * 1000) / elapsed / 1_000_000 : 0;

// Each worker reports cumulative decimal Mbps at roughly aligned intervals.
// Parallel throughput is their sum, carrying a completed worker's last sample
// forward when another worker emitted one additional interval.
export const aggregateMbpsSamples = (workers: number[][]) => {
  const length = Math.max(0, ...workers.map((samples) => samples.length));
  return Array.from({ length }, (_, index) =>
    workers.reduce((total, samples) => {
      if (samples.length === 0) return total;
      return total + samples[Math.min(index, samples.length - 1)];
    }, 0),
  );
};

/** Real browser transport: every byte counted from a reader; uploads count only JSON acknowledgements. */
export function createBrowserTransport(
  runID: string,
  fetcher: Fetcher = fetch,
  now = () => performance.now(),
  onLiveSample?: (sample: LiveSpeedSample) => void,
): Transport {
  const url = (path: string) => `/api/test-runs/${runID}/${path}`;
  async function downloadWorker(
    parent: AbortSignal,
    progress: {
      bytes: number;
      samples: number[];
      incomplete: boolean;
      settled: boolean;
      frozen: boolean;
    },
  ): Promise<void> {
    const ctl = new AbortController();
    const watchdogEnded = Symbol("download watchdog ended");
    let endWatchdog!: () => void;
    const watchdogStop = new Promise<typeof watchdogEnded>((resolve) => {
      endWatchdog = () => resolve(watchdogEnded);
    });
    const cancel = () => {
      ctl.abort();
      endWatchdog();
    };
    if (parent.aborted) cancel();
    else parent.addEventListener("abort", cancel, { once: true });
    const watchdog = window.setTimeout(cancel, DOWNLOAD_TIMEOUT_MS);
    const signal = ctl.signal;
    const started = now();
    const markIncomplete = () => {
      if (parent.aborted) return;
      progress.incomplete = true;
      if (!progress.frozen && progress.bytes > 0) {
        const elapsed = now() - started;
        progress.samples.push(mbps(progress.bytes, elapsed));
      }
    };
    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    let complete = false;
    try {
      const response = await Promise.race([
        fetcher(
          `${url("download")}?nonce=${crypto.randomUUID()}&durationMs=${PHASE_MS}`,
          { signal, cache: "no-store" },
        ),
        watchdogStop,
      ]);
      if (response === watchdogEnded) {
        markIncomplete();
        return;
      }
      if (!response.ok || !response.body) throw new Error("download failed");
      reader = response.body.getReader();
      for (;;) {
        const x = await Promise.race([reader.read(), watchdogStop]);
        if (x === watchdogEnded) {
          markIncomplete();
          break;
        }
        if (x.done) {
          complete = true;
          break;
        }
        if (!progress.frozen) progress.bytes += x.value.byteLength;
        const elapsed = now() - started;
        if (!progress.frozen && elapsed >= (progress.samples.length + 1) * 250)
          progress.samples.push(mbps(progress.bytes, elapsed));
        if (elapsed >= PHASE_MS) break;
      }
    } catch (error) {
      if (!signal.aborted) throw error;
    } finally {
      window.clearTimeout(watchdog);
      parent.removeEventListener("abort", cancel);
      if (reader && !complete) {
        let drainTimeout: number | undefined;
        await Promise.race([
          reader.cancel().catch(() => undefined),
          new Promise<void>((resolve) => {
            drainTimeout = window.setTimeout(resolve, 250);
          }),
        ]);
        if (drainTimeout !== undefined) window.clearTimeout(drainTimeout);
      }
      if (
        !progress.frozen &&
        !parent.aborted &&
        progress.bytes > 0 &&
        progress.samples.length === 0
      ) {
        const final = mbps(progress.bytes, now() - started);
        if (final > 0) progress.samples.push(final);
      }
      progress.settled = true;
    }
  }
  async function uploadWorker(
    parent: AbortSignal,
    progress?: { ackBytes: number },
  ): Promise<{ ackBytes: number; samples: number[] }> {
    const ctl = new AbortController();
    const phaseEnded = Symbol("phase ended");
    let endPhase!: () => void;
    const phaseStop = new Promise<typeof phaseEnded>((resolve) => {
      endPhase = () => resolve(phaseEnded);
    });
    const cancel = () => {
      ctl.abort();
      endPhase();
    };
    const timeout = window.setTimeout(cancel, PHASE_MS);
    if (parent.aborted) cancel();
    else parent.addEventListener("abort", cancel, { once: true });
    const signal = ctl.signal;
    const started = now();
    let ackBytes = 0;
    const samples: number[] = [];
    try {
      for (
        let i = 0;
        i < MAX_UPLOAD_REQUESTS &&
        !signal.aborted &&
        now() - started < PHASE_MS;
        i++
      ) {
        const response = await Promise.race([
          fetcher(url("upload"), {
            method: "POST",
            body: new Uint8Array(CHUNK),
            signal,
            cache: "no-store",
          }),
          phaseStop,
        ]);
        if (response === phaseEnded) break;
        if (!response.ok) throw new Error("upload failed");
        const ack = await Promise.race([response.json(), phaseStop]);
        if (ack === phaseEnded) break;
        const acknowledged = ack as { bytes?: number };
        ackBytes += Number(acknowledged.bytes) || 0;
        if (progress) progress.ackBytes = ackBytes;
        const elapsed = now() - started;
        if (elapsed >= (samples.length + 1) * 250)
          samples.push(mbps(ackBytes, elapsed));
      }
    } catch (error) {
      if (!signal.aborted) throw error;
    } finally {
      window.clearTimeout(timeout);
      parent.removeEventListener("abort", cancel);
    }
    const final = mbps(ackBytes, now() - started);
    if (final > 0 && samples.length === 0) samples.push(final);
    return { ackBytes, samples };
  }
  return {
    now,
    async download(streams, signal) {
      const phase = new AbortController();
      const cancel = () => phase.abort();
      if (signal.aborted) cancel();
      else signal.addEventListener("abort", cancel, { once: true });
      const progress = Array.from({ length: streams }, () => ({
        bytes: 0,
        samples: [] as number[],
        incomplete: false,
        settled: false,
        frozen: false,
      }));
      const liveStarted = now();
      const emitLive = () => {
        const bytes = progress.reduce((total, item) => total + item.bytes, 0);
        const rate = mbps(bytes, now() - liveStarted);
        if (rate > 0)
          onLiveSample?.({ at: now(), direction: "download", mbps: rate });
      };
      const liveTimer = onLiveSample
        ? window.setInterval(emitLive, 250)
        : undefined;
      const workers = progress.map((state) =>
        downloadWorker(phase.signal, state),
      );
      const all = Promise.all(workers);
      const aggregateEnded = Symbol("download aggregate ended");
      let aggregateTimeout: number | undefined;
      const aggregateStop = new Promise<typeof aggregateEnded>((resolve) => {
        aggregateTimeout = window.setTimeout(
          () => resolve(aggregateEnded),
          DOWNLOAD_AGGREGATE_TIMEOUT_MS,
        );
      });
      let outcome: void[] | typeof aggregateEnded;
      try {
        outcome = await Promise.race([all, aggregateStop]);
      } catch (error) {
        for (const state of progress) state.frozen = true;
        phase.abort();
        void all.catch(() => undefined);
        throw error;
      } finally {
        if (liveTimer !== undefined) window.clearInterval(liveTimer);
        emitLive();
        if (aggregateTimeout !== undefined)
          window.clearTimeout(aggregateTimeout);
        signal.removeEventListener("abort", cancel);
      }
      for (const state of progress) state.frozen = true;
      if (outcome === aggregateEnded) {
        phase.abort();
        void all.catch(() => undefined);
      }
      return {
        bytes: progress.reduce((n, x) => n + x.bytes, 0),
        samples: aggregateMbpsSamples(progress.map((x) => x.samples)),
        incompleteStreams: progress.filter((x) => x.incomplete || !x.settled)
          .length,
      };
    },
    async upload(streams, signal) {
      const progress = Array.from({ length: streams }, () => ({ ackBytes: 0 }));
      const liveStarted = now();
      const emitLive = () => {
        const bytes = progress.reduce(
          (total, item) => total + item.ackBytes,
          0,
        );
        const rate = mbps(bytes, now() - liveStarted);
        if (rate > 0)
          onLiveSample?.({ at: now(), direction: "upload", mbps: rate });
      };
      const liveTimer = onLiveSample
        ? window.setInterval(emitLive, 250)
        : undefined;
      let all: { ackBytes: number; samples: number[] }[] = [];
      try {
        all = await Promise.all(
          progress.map((state) => uploadWorker(signal, state)),
        );
      } finally {
        if (liveTimer !== undefined) window.clearInterval(liveTimer);
        emitLive();
      }
      return {
        ackBytes: all.reduce((n, x) => n + x.ackBytes, 0),
        samples: aggregateMbpsSamples(all.map((x) => x.samples)),
      };
    },
  };
}
