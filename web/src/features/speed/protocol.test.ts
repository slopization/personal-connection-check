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
});
