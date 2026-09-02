export function isUnsupportedWebKit(
  userAgent = typeof navigator === "undefined" ? "" : navigator.userAgent,
): boolean {
  return (
    /AppleWebKit\//.test(userAgent) &&
    !/(?:Chrome|Chromium|Edg|OPR)\//.test(userAgent)
  );
}
