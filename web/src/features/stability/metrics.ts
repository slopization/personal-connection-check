export type Sample = { at: number; rtt: number };
export function jitter(samples: Sample[]): number {
  const a = samples.slice(-30);
  if (a.length < 2) return 0;
  return (
    a.slice(1).reduce((n, x, i) => n + Math.abs(x.rtt - a[i].rtt), 0) /
    (a.length - 1)
  );
}
export function range(
  samples: Sample[],
  which: "10m" | "1h" | "all",
  now = Date.now(),
) {
  const cutoff =
    which === "10m" ? now - 600000 : which === "1h" ? now - 3600000 : 0;
  return samples.filter((x) => x.at >= cutoff);
}
