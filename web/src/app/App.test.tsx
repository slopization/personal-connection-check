import { afterEach, describe, expect, it, vi } from "vitest";
import {
  closeRun,
  downloadBlob,
  warnIncompleteDownload,
  warmPing,
} from "./App";

describe("run cleanup", () => {
  it("deletes the owned run with a keepalive request", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));

    expect(await closeRun("run/id", fetcher)).toBe(true);

    expect(fetcher).toHaveBeenCalledWith(
      "/api/test-runs/run%2Fid",
      expect.objectContaining({ method: "DELETE", keepalive: true }),
    );
  });
});

describe("incomplete download diagnostics", () => {
  it("logs incomplete streams and reports whether a user warning is needed", () => {
    const logger = vi.fn();

    expect(warnIncompleteDownload(2, logger)).toBe(true);
    expect(logger).toHaveBeenCalledWith(
      "Download measurement used partial stream snapshots",
      { incompleteStreams: 2 },
    );
    expect(warnIncompleteDownload(0, logger)).toBe(false);
    expect(logger).toHaveBeenCalledOnce();
  });
});

describe("PNG download", () => {
  afterEach(() => vi.useRealTimers());

  it("clicks a connected download anchor and delays Blob URL cleanup", () => {
    vi.useFakeTimers();
    const blob = new Blob(["png"], { type: "image/png" });
    const createObjectURL = vi.fn(() => "blob:pcc-result");
    const revokeObjectURL = vi.fn();
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined);

    downloadBlob(blob, document, createObjectURL, revokeObjectURL);

    const anchor = document.querySelector<HTMLAnchorElement>(
      'a[download="connection-check.png"]',
    );
    expect(createObjectURL).toHaveBeenCalledWith(blob);
    expect(anchor).not.toBeNull();
    expect(anchor?.isConnected).toBe(true);
    expect(anchor?.href).toBe("blob:pcc-result");
    expect(click).toHaveBeenCalledOnce();
    expect(revokeObjectURL).not.toHaveBeenCalled();

    vi.advanceTimersByTime(999);
    expect(anchor?.isConnected).toBe(true);
    vi.advanceTimersByTime(1);
    expect(anchor?.isConnected).toBe(false);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:pcc-result");
    click.mockRestore();
  });
});

describe("warm ping", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("measures median HTTP health-check RTT after warm-up", async () => {
    let clock = 0;
    const fetcher = vi.fn(
      async (_url: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 200 }),
    );

    const ping = await warmPing(
      new AbortController().signal,
      10_000,
      fetcher,
      () => (clock += 5),
    );

    expect(ping).toBe(5);
    expect(fetcher).toHaveBeenCalledTimes(8);
    expect(String(fetcher.mock.calls[0][0])).toContain("/healthz?ping=0");
  });

  it("aborts an active HTTP request when cancelled during warm-up", async () => {
    const fetcher = vi.fn(
      (_url: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) =>
          init?.signal?.addEventListener(
            "abort",
            () => reject(new DOMException("Aborted", "AbortError")),
            { once: true },
          ),
        ),
    );
    const controller = new AbortController();
    const pending = warmPing(controller.signal, 10_000, fetcher);

    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
  });

  it("closes and rejects when warm-up exceeds its bounded timeout", async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn(
      (_url: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) =>
          init?.signal?.addEventListener(
            "abort",
            () => reject(new DOMException("Aborted", "AbortError")),
            { once: true },
          ),
        ),
    );
    const pending = warmPing(new AbortController().signal, 100, fetcher);
    const rejected = expect(pending).rejects.toThrow("Ping timed out");

    await vi.advanceTimersByTimeAsync(100);

    await rejected;
  });
});
