import { afterEach, describe, expect, it, vi } from "vitest";
import { warmPing } from "./App";

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
