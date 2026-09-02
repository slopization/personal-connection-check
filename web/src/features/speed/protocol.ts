import type { Phase, Transport } from "./engine";

type Fetcher = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;
const CHUNK = 1024 * 1024;
const PHASE_MS = 1250;
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
    const timeout = window.setTimeout(() => ctl.abort(), 1250);
    parent.addEventListener("abort", () => ctl.abort(), { once: true });
    const signal = ctl.signal;
    const started = now();
    const response = await fetcher(
      `${url("download")}?nonce=${crypto.randomUUID()}`,
      { signal, cache: "no-store" },
    );
    if (!response.ok || !response.body) throw new Error("download failed");
    const reader = response.body.getReader();
    let bytes = 0;
    const samples: number[] = [];
    try {
      for (;;) {
        const x = await reader.read();
        if (x.done) break;
        bytes += x.value.byteLength;
        const elapsed = now() - started;
        if (elapsed >= (samples.length + 1) * 250)
          samples.push(mbps(bytes, elapsed));
      }
    } catch (error) {
      if (!signal.aborted) throw error;
    } finally {
      window.clearTimeout(timeout);
      reader.cancel().catch(() => undefined);
    }
    return { bytes, samples };
  }
  async function uploadWorker(
    parent: AbortSignal,
  ): Promise<{ ackBytes: number; samples: number[] }> {
    const ctl = new AbortController();
    const timeout = window.setTimeout(() => ctl.abort(), PHASE_MS);
    const cancel = () => ctl.abort();
    parent.addEventListener("abort", cancel, { once: true });
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
        const response = await fetcher(url("upload"), {
          method: "POST",
          body: new Uint8Array(CHUNK),
          signal,
          cache: "no-store",
        });
        if (!response.ok) throw new Error("upload failed");
        const ack = (await response.json()) as { bytes?: number };
        ackBytes += Number(ack.bytes) || 0;
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
