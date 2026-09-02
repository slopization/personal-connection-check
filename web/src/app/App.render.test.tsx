import "fake-indexeddb/auto";
import { h, render } from "preact";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

async function loggedInApp(userAgent?: string) {
  vi.resetModules();
  Object.defineProperty(navigator, "language", {
    configurable: true,
    value: "ko-KR",
  });
  if (userAgent) {
    Object.defineProperty(navigator, "userAgent", {
      configurable: true,
      value: userAgent,
    });
    Object.defineProperty(navigator, "vendor", {
      configurable: true,
      value: "Apple Computer, Inc.",
    });
  }
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/auth/config") {
        return new Response(
          JSON.stringify({ password: true, oidc: false, footerMessage: "" }),
        );
      }
      return new Response(JSON.stringify({ ip: "127.0.0.1" }));
    }),
  );
  const { App } = await import("./App");
  const host = document.createElement("div");
  document.body.append(host);
  render(h(App, {}), host);
  await vi.waitFor(() => expect(host.querySelector("button")).not.toBeNull());
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

  it("shows only OIDC login when shared-password authentication is disabled", async () => {
    vi.resetModules();
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ko-KR",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/api/auth/config") {
          return new Response(
            JSON.stringify({ password: false, oidc: true, footerMessage: "" }),
          );
        }
        return new Response(null, { status: 401 });
      }),
    );
    const { App } = await import("./App");
    const host = document.createElement("div");
    document.body.append(host);
    render(h(App, {}), host);

    await vi.waitFor(() =>
      expect(
        host.querySelector<HTMLAnchorElement>('a[href="/api/auth/oidc/begin"]'),
      ).not.toBeNull(),
    );
    expect(host.querySelector('input[type="password"]')).toBeNull();
    expect(host.textContent).not.toContain("공유 비밀번호");
  });

  it("restores an existing OIDC session after the callback redirect", async () => {
    vi.resetModules();
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ko-KR",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "/api/auth/config") {
          return new Response(
            JSON.stringify({ password: false, oidc: true, footerMessage: "" }),
          );
        }
        if (url === "/api/network-info") {
          return new Response(JSON.stringify({ ip: "127.0.0.1" }));
        }
        return new Response(null, { status: 404 });
      }),
    );
    const { App } = await import("./App");
    const host = document.createElement("div");
    document.body.append(host);
    render(h(App, {}), host);

    await vi.waitFor(() => expect(host.textContent).toContain("측정 시작"));
    expect(host.querySelector('a[href="/api/auth/oidc/begin"]')).toBeNull();
  });

  it("shows the configured footer message before and after login", async () => {
    vi.resetModules();
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ko-KR",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "/api/auth/config") {
          return new Response(
            JSON.stringify({
              password: true,
              oidc: false,
              footerMessage: "IP Geolocation by DB-IP: https://db-ip.com/",
            }),
          );
        }
        if (url === "/api/network-info") {
          return new Response(JSON.stringify({ ip: "127.0.0.1" }));
        }
        return new Response(null, { status: 204 });
      }),
    );
    const { App } = await import("./App");
    const host = document.createElement("div");
    document.body.append(host);
    render(h(App, {}), host);

    await vi.waitFor(() =>
      expect(host.querySelector("footer")?.textContent).toContain(
        "IP Geolocation by DB-IP",
      ),
    );
    expect(host.querySelector<HTMLAnchorElement>("footer a")?.href).toBe(
      "https://db-ip.com/",
    );

    (host.querySelector("button") as HTMLButtonElement).click();
    await flush();
    expect(host.querySelector("footer")?.textContent).toContain(
      "IP Geolocation by DB-IP",
    );
  });

  it("renders footer URLs as links without interpreting HTML", async () => {
    const { FooterMessage } = await import("./App");
    const host = document.createElement("div");
    document.body.append(host);

    render(
      h(FooterMessage, {
        message:
          "IP Geolocation by DB-IP: https://db-ip.com/ <script>alert(1)</script>",
      }),
      host,
    );

    expect(host.querySelector("a")?.href).toBe("https://db-ip.com/");
    expect(host.querySelector("script")).toBeNull();
    expect(host.textContent).toContain("<script>alert(1)</script>");
  });

  it("renders an explicit partial-stream accuracy warning", async () => {
    const { IncompleteDownloadWarning } = await import("./App");
    const host = document.createElement("div");
    document.body.append(host);

    render(h(IncompleteDownloadWarning, { count: 2 }), host);

    expect(host.querySelector('[role="alert"]')?.textContent).toContain(
      "다운로드 스트림 2개가 정상적으로 끝나지 않았습니다",
    );
  });

  it("warns mobile Safari without disabling measurement", async () => {
    const { clear } = await import("../storage/database");
    await clear();
    const host = await loggedInApp(
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1",
    );

    expect(host.querySelector('[role="alert"]')?.textContent).toContain(
      "Safari 및 WebKit 계열 브라우저는 지원되지 않습니다",
    );
    expect(
      [...host.querySelectorAll("button")].find(
        (button) => button.textContent === "측정 시작",
      )?.disabled,
    ).toBe(false);
  });
});
