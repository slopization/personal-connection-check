import { describe, expect, it } from "vitest";
import { isUnsupportedWebKit } from "./browser";

describe("Safari and WebKit support warning", () => {
  it("matches desktop and mobile WebKit but not Blink or Gecko", () => {
    const desktopSafari =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15";
    const mobileSafari =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1";
    const chrome =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36";
    const ipadDesktopMode =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15";
    const iosChrome =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/139.0.0.0 Mobile/15E148 Safari/604.1";
    const webKitGTK =
      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15";
    const firefox =
      "Mozilla/5.0 (X11; Linux x86_64; rv:142.0) Gecko/20100101 Firefox/142.0";

    expect(isUnsupportedWebKit(desktopSafari)).toBe(true);
    expect(isUnsupportedWebKit(mobileSafari)).toBe(true);
    expect(isUnsupportedWebKit(ipadDesktopMode)).toBe(true);
    expect(isUnsupportedWebKit(iosChrome)).toBe(true);
    expect(isUnsupportedWebKit(webKitGTK)).toBe(true);
    expect(isUnsupportedWebKit(chrome)).toBe(false);
    expect(isUnsupportedWebKit(firefox)).toBe(false);
  });
});
