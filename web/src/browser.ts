export function isDesktopSafari(
  userAgent = typeof navigator === "undefined" ? "" : navigator.userAgent,
  vendor = typeof navigator === "undefined" ? "" : navigator.vendor,
  maxTouchPoints = typeof navigator === "undefined"
    ? 0
    : (navigator.maxTouchPoints ?? 0),
): boolean {
  return (
    vendor === "Apple Computer, Inc." &&
    maxTouchPoints === 0 &&
    /Safari\//.test(userAgent) &&
    !/Mobile\//.test(userAgent) &&
    !/(?:Chrome|Chromium|CriOS|FxiOS|Edg|OPR)\//.test(userAgent)
  );
}
