import { afterEach, describe, expect, it, vi } from "vitest";
import { blobBase64, closeRun, submitPNG, warmPing } from "./App";

class PendingSocket {
  static instances: PendingSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  send = vi.fn();

  constructor(..._args: unknown[]) {
    PendingSocket.instances.push(this);
  }
}

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

describe("PNG download", () => {
  it("encodes a PNG blob for the native attachment form", async () => {
    await expect(blobBase64(new Blob(["png"]))).resolves.toBe("cG5n");
  });

  it("submits a connected same-origin form for WebKit native attachment", () => {
    vi.useFakeTimers();
    const submit = vi
      .spyOn(HTMLFormElement.prototype, "submit")
      .mockImplementation(function (this: HTMLFormElement) {
        expect(this.isConnected).toBe(true);
        expect(this.method).toBe("post");
        expect(this.action).toMatch(/\/api\/share\.png$/);
        expect(new FormData(this).get("data")).toBe("cG5n");
      });

    submitPNG("cG5n", document);

    expect(submit).toHaveBeenCalledOnce();
    expect(
      document.querySelector('form[action="/api/share.png"]'),
    ).not.toBeNull();
    vi.runAllTimers();
    expect(document.querySelector('form[action="/api/share.png"]')).toBeNull();
    submit.mockRestore();
    vi.useRealTimers();
  });
});

describe("warm ping", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    PendingSocket.instances = [];
  });

  it("closes and rejects immediately when cancelled during warm-up", async () => {
    vi.stubGlobal("WebSocket", PendingSocket);
    const controller = new AbortController();
    const pending = warmPing(controller.signal, 10_000);

    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
    expect(PendingSocket.instances[0].close).toHaveBeenCalledOnce();
  });

  it("closes and rejects when warm-up exceeds its bounded timeout", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", PendingSocket);
    const pending = warmPing(new AbortController().signal, 100);
    const rejected = expect(pending).rejects.toThrow("Ping timed out");

    await vi.advanceTimersByTimeAsync(100);

    await rejected;
    expect(PendingSocket.instances[0].close).toHaveBeenCalledOnce();
  });
});
