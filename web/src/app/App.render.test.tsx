import "fake-indexeddb/auto";
import { h, render } from "preact";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

async function loggedInApp() {
  vi.resetModules();
  Object.defineProperty(navigator, "language", {
    configurable: true,
    value: "ko-KR",
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ ip: "127.0.0.1" }))),
  );
  const { App } = await import("./App");
  const host = document.createElement("div");
  document.body.append(host);
  render(h(App, {}), host);
  (host.querySelector("button") as HTMLButtonElement).click();
  await flush();
  return host;
}

describe("stability restoration and Korean UI", () => {
  beforeEach(() => {
    document.body.replaceChildren();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("hydrates saved stability samples and pause markers when entering Stability", async () => {
    const { clear, saveStability } = await import("../storage/database");
    await clear();
    await saveStability({
      at: 100,
      samples: [{ at: 100, rtt: 5 }],
      pauses: [99],
    });
    const host = await loggedInApp();

    [...host.querySelectorAll("button")]
      .find((button) => button.textContent === "안정성")
      ?.click();
    await flush();

    expect(host.querySelector("time")?.dateTime).toBe(
      new Date(99).toISOString(),
    );
    expect(host.textContent).not.toContain("No samples");
  });

  it("renders Korean navigation, empty chart, and pause labels", async () => {
    const { clear } = await import("../storage/database");
    await clear();
    const host = await loggedInApp();

    [...host.querySelectorAll("button")]
      .find((button) => button.textContent === "안정성")
      ?.click();
    await flush();

    expect(host.textContent).toContain("안정성");
    expect(host.textContent).toContain("측정 일시정지");
    expect(host.textContent).toContain("샘플 없음");
    expect(host.textContent).not.toContain("No samples");
  });
});
