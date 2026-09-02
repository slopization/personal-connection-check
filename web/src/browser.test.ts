import { describe, expect, it } from "vitest";
import { isDesktopSafari } from "./browser";

describe("desktop Safari support warning", () => {
  it("matches desktop Safari but not mobile Safari or other desktop browsers", () => {
    const desktopSafari =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15";
    const mobileSafari =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1";
    const chrome =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36";
    const ipadDesktopMode =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15";

    expect(isDesktopSafari(desktopSafari, "Apple Computer, Inc.", 0)).toBe(
      true,
    );
    expect(isDesktopSafari(mobileSafari, "Apple Computer, Inc.", 5)).toBe(
      false,
    );
    expect(isDesktopSafari(ipadDesktopMode, "Apple Computer, Inc.", 5)).toBe(
      false,
    );
    expect(isDesktopSafari(chrome, "Google Inc.", 0)).toBe(false);
  });
});
