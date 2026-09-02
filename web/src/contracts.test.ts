import { describe, it, expect } from "vitest";
import { language, text } from "./i18n";
import { jitter, range } from "./features/stability/metrics";
import { median, stable } from "./features/speed/engine";
describe("client contracts", () => {
  it("selects Korean only for ko locales", () => {
    expect(language("ko-KR")).toBe("ko");
    expect(language("ja")).toBe("en");
    expect(text.ko.start).toBe("측정 시작");
  });
  it("calculates jitter and display ranges without mutation", () => {
    const a = [
      { at: 0, rtt: 10 },
      { at: 1, rtt: 30 },
      { at: 2, rtt: 20 },
    ];
    expect(jitter(a)).toBe(15);
    expect(range(a, "all")).toEqual(a);
    expect(median([3, 1, 2])).toBe(2);
    expect(stable([100, 104])).toBe(true);
  });
});
