import { afterEach, describe, expect, it, vi } from "vitest";
import { closeRun, downloadBlob, warmPing } from "./App";

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
