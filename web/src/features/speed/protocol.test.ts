import { describe, expect, it, vi } from "vitest";
import { createBrowserTransport } from "./protocol";

describe("browser speed transport", () => {
  it("uses parallel download readers and server acknowledged repeated upload chunks", async () => {
    const fetcher = vi.fn(
      async (url: RequestInfo | URL, init?: RequestInit) => {
        if (String(url).includes("download"))
          return new Response(new Uint8Array(64));
        return new Response(JSON.stringify({ bytes: 32 }), {
          headers: { "content-type": "application/json" },
        });
      },
    );
    const transport = createBrowserTransport("run", fetcher, () => 1000);
    const download = await transport.download(2, new AbortController().signal);
    const upload = await transport.upload(2, new AbortController().signal);
    expect(download.bytes).toBe(128);
    expect(upload.ackBytes).toBeGreaterThanOrEqual(64);
    expect(
      fetcher.mock.calls.filter(([url]) => String(url).includes("download")),
    ).toHaveLength(2);
    expect(
      fetcher.mock.calls.filter(([url]) => String(url).includes("upload"))
        .length,
    ).toBeGreaterThanOrEqual(2);
  });

  it("keeps upload workers active for a bounded phase and emits timed samples", async () => {
    let clock = 0;
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify({ bytes: 32 }), {
          headers: { "content-type": "application/json" },
        }),
    );
    const transport = createBrowserTransport(
      "run",
      fetcher,
      () => (clock += 100),
    );

    const upload = await transport.upload(1, new AbortController().signal);

    expect(upload.samples.length).toBeGreaterThanOrEqual(4);
    expect(upload.ackBytes).toBeGreaterThanOrEqual(192);
    expect(fetcher).toHaveBeenCalledTimes(6);
  });

  it("waits for download cancellation to release server stream slots", async () => {
    let finishRead!: () => void;
    let finishCancel!: () => void;
    let readBlocked!: () => void;
    const blocked = new Promise<void>((resolve) => (readBlocked = resolve));
    const cancel = vi.fn(
      () => new Promise<void>((resolve) => (finishCancel = resolve)),
    );
    const fetcher = vi.fn(
      async (_url: RequestInfo | URL, init?: RequestInit) => {
        let reads = 0;
        init?.signal?.addEventListener("abort", () => finishRead(), {
          once: true,
        });
        return {
          ok: true,
          body: {
            getReader: () => ({
              read: () => {
                if (reads++ === 0)
                  return Promise.resolve({
                    done: false,
                    value: new Uint8Array(8),
                  });
                return new Promise<{ done: true; value?: undefined }>(
                  (resolve) => {
                    finishRead = () => resolve({ done: true });
                    readBlocked();
                  },
                );
              },
              cancel,
            }),
          },
        } as unknown as Response;
      },
    );
    const transport = createBrowserTransport("run", fetcher, () => 1000);
    const ctl = new AbortController();
    let settled = false;
    const result = transport.download(1, ctl.signal).finally(() => {
      settled = true;
    });

    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
    await blocked;
    ctl.abort();
    await vi.waitFor(() => expect(cancel).toHaveBeenCalledOnce());
    expect(settled).toBe(false);
    finishCancel();
    await result;
    expect(settled).toBe(true);
  });
});
