import { describe, expect, it, vi } from "vitest";
import { createBrowserTransport } from "./protocol";

describe("browser speed transport", () => {
  it("bounds an upload when WebKit fetch ignores abort", async () => {
    vi.useFakeTimers();
    const fetcher: typeof fetch = vi.fn(
      () => new Promise<Response>(() => undefined),
    );
    const transport = createBrowserTransport("run", fetcher, () => 0);
    let settled = false;
    const result = transport
      .upload(1, new AbortController().signal)
      .then((value) => {
        settled = true;
        return value;
      });

    await vi.advanceTimersByTimeAsync(1_250);
    expect(settled).toBe(true);
    expect((await result).ackBytes).toBe(0);
    vi.useRealTimers();
  });

  it("bounds a download when WebKit reader.read never settles", async () => {
    vi.useFakeTimers();
    const cancel = vi.fn(async () => undefined);
    const reader = {
      read: () => new Promise<never>(() => undefined),
      cancel,
    } as unknown as ReadableStreamDefaultReader<Uint8Array>;
    const fetcher: typeof fetch = vi.fn(
      async () =>
        ({
          ok: true,
          body: { getReader: () => reader },
        }) as unknown as Response,
    );
    const transport = createBrowserTransport("run", fetcher, () => 0);
    let settled = false;
    const result = transport
      .download(1, new AbortController().signal)
      .then((value) => {
        settled = true;
        return value;
      });

    await vi.advanceTimersByTimeAsync(1_250);
    expect(settled).toBe(true);
    expect((await result).bytes).toBe(0);
    expect(cancel).toHaveBeenCalledOnce();
    vi.useRealTimers();
  });

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
                  (_resolve, reject) => {
                    finishRead = () =>
                      reject(new DOMException("cancelled", "AbortError"));
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

  it("does not cancel a download reader after normal EOF", async () => {
    const cancel = vi.fn(() => new Promise<void>(() => undefined));
    const fetcher = vi.fn(
      async () =>
        ({
          ok: true,
          body: {
            getReader: () => ({
              read: async () => ({ done: true, value: undefined }),
              cancel,
            }),
          },
        }) as unknown as Response,
    );
    const transport = createBrowserTransport("run", fetcher, () => 1000);

    const settled = await Promise.race([
      transport.download(1, new AbortController().signal).then(() => true),
      new Promise<false>((resolve) => setTimeout(() => resolve(false), 50)),
    ]);

    expect(settled).toBe(true);
    expect(cancel).not.toHaveBeenCalled();
  });

  it("stops reading buffered download data at the phase wall clock", async () => {
    let reads = 0;
    let clock = 0;
    const cancel = vi.fn(async () => undefined);
    const fetcher = vi.fn(
      async () =>
        ({
          ok: true,
          body: {
            getReader: () => ({
              read: async () => {
                if (++reads > 10) throw new Error("drained buffered backlog");
                return { done: false, value: new Uint8Array(8) };
              },
              cancel,
            }),
          },
        }) as unknown as Response,
    );
    const transport = createBrowserTransport(
      "run",
      fetcher,
      () => (clock += 500),
    );

    await expect(
      transport.download(1, new AbortController().signal),
    ).resolves.toMatchObject({ bytes: expect.any(Number) });
    expect(reads).toBeLessThanOrEqual(3);
    expect(cancel).toHaveBeenCalledOnce();
  });
});
