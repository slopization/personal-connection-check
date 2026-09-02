import type { Phase, Transport } from "./engine";

type Fetcher = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;
const CHUNK = 1024 * 1024;
const PHASE_MS = 1250;
const DOWNLOAD_TIMEOUT_MS = PHASE_MS + 500;
const MAX_UPLOAD_REQUESTS = 8;
const mbps = (bytes: number, elapsed: number) =>
  elapsed > 0 ? (bytes * 8 * 1000) / elapsed / 1_000_000 : 0;

/** Real browser transport: every byte counted from a reader; uploads count only JSON acknowledgements. */
export function createBrowserTransport(
  runID: string,
  fetcher: Fetcher = fetch,
  now = () => performance.now(),
): Transport {
  const url = (path: string) => `/api/test-runs/${runID}/${path}`;
  async function downloadWorker(parent: AbortSignal): Promise<Phase> {
    const ctl = new AbortController();
    const cancel = () => ctl.abort();
    parent.addEventListener("abort", cancel, { once: true });
    const timeout = window.setTimeout(cancel, DOWNLOAD_TIMEOUT_MS);
    const phaseEnded = Symbol("phase ended");
    let phaseTimeout: number | undefined;
    const phaseStop = new Promise<typeof phaseEnded>((resolve) => {
      phaseTimeout = window.setTimeout(() => {
        cancel();
        resolve(phaseEnded);
      }, PHASE_MS);
    });
    const signal = ctl.signal;
    const started = now();
    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    let bytes = 0;
    const samples: number[] = [];
    let complete = false;
    try {
      const response = await Promise.race([
        fetcher(
          `${url("download")}?nonce=${crypto.randomUUID()}&durationMs=${PHASE_MS}`,
          { signal, cache: "no-store" },
        ),
        phaseStop,
      ]);
      if (response === phaseEnded) return { bytes, samples };
      if (!response.ok || !response.body) throw new Error("download failed");
      reader = response.body.getReader();
      for (;;) {
        const x = await Promise.race([reader.read(), phaseStop]);
        if (x === phaseEnded) break;
        if (x.done) {
          complete = true;
          break;
        }
        bytes += x.value.byteLength;
        const elapsed = now() - started;
        if (elapsed >= (samples.length + 1) * 250)
          samples.push(mbps(bytes, elapsed));
        if (elapsed >= PHASE_MS) break;
      }
    } catch (error) {
      if (!signal.aborted) throw error;
    } finally {
      window.clearTimeout(timeout);
      if (phaseTimeout !== undefined) window.clearTimeout(phaseTimeout);
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
    }
    return { bytes, samples };
  }
  async function uploadWorker(
    parent: AbortSignal,
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
    return { ackBytes, samples };
  }
  return {
    now,
    async download(streams, signal) {
      const all = await Promise.all(
        Array.from({ length: streams }, () => downloadWorker(signal)),
      );
      return {
        bytes: all.reduce((n, x) => n + x.bytes, 0),
        samples: all.flatMap((x) => x.samples),
      };
    },
    async upload(streams, signal) {
      const all = await Promise.all(
        Array.from({ length: streams }, () => uploadWorker(signal)),
      );
      return {
        ackBytes: all.reduce((n, x) => n + x.ackBytes, 0),
        samples: all.flatMap((x) => x.samples),
      };
    },
  };
}
