import { useEffect, useRef } from "preact/hooks";
import type uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import { range, type Sample } from "./metrics";
export type ChartRange = "10m" | "1h" | "all";
export function filteredSamples(
  samples: Sample[],
  selected: ChartRange,
  now = Date.now(),
) {
  return range(samples, selected, now);
}
type ChartLabels = { aria: string; rtt: string; empty: string };
export function StabilityChart({
  samples,
  selected,
  labels,
}: {
  samples: Sample[];
  selected: ChartRange;
  labels: ChartLabels;
}) {
  const host = useRef<HTMLDivElement>(null);
  const shown = filteredSamples(samples, selected);
  useEffect(() => {
    let chart: uPlot | undefined;
    let dead = false;
    if (!host.current || !shown.length) return;
    void import("uplot").then(({ default: Plot }) => {
      if (!dead && host.current)
        chart = new Plot(
          {
            width: Math.max(280, host.current.clientWidth),
            height: 220,
            series: [{}, { label: labels.rtt, stroke: "#4c9" }],
          },
          [shown.map((x) => x.at / 1000), shown.map((x) => x.rtt)],
          host.current,
        );
    });
    return () => {
      dead = true;
      chart?.destroy();
    };
  }, [labels.rtt, shown]);
  return (
    <div aria-label={labels.aria} ref={host}>
      {shown.length === 0 && <p>{labels.empty}</p>}
    </div>
  );
}
