import type { LiveSpeedSample } from "./protocol";

type Labels = {
  aria: string;
  download: string;
  upload: string;
  empty: string;
};

export function SpeedChart({
  samples,
  labels,
}: {
  samples: LiveSpeedSample[];
  labels: Labels;
}) {
  const shown = samples.slice(-160);
  if (shown.length === 0)
    return (
      <div class="speed-chart-empty" aria-label={labels.aria}>
        {labels.empty}
      </div>
    );
  const start = shown[0].at;
  const end = Math.max(start + 1, shown.at(-1)!.at);
  const ceiling = Math.max(10, ...shown.map((sample) => sample.mbps)) * 1.1;
  const points = (direction: LiveSpeedSample["direction"]) =>
    shown
      .filter((sample) => sample.direction === direction)
      .map((sample) => {
        const x = 20 + ((sample.at - start) / (end - start)) * 560;
        const y = 190 - (sample.mbps / ceiling) * 160;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
      })
      .join(" ");
  return (
    <figure class="speed-chart" aria-label={labels.aria}>
      <svg viewBox="0 0 600 220" role="img">
        <line x1="20" y1="190" x2="580" y2="190" class="chart-axis" />
        <line x1="20" y1="30" x2="20" y2="190" class="chart-axis" />
        <text x="24" y="24" class="chart-scale">
          {ceiling.toFixed(0)} Mbps
        </text>
        <polyline points={points("download")} class="speed-line download" />
        <polyline points={points("upload")} class="speed-line upload" />
      </svg>
      <figcaption>
        <span class="legend-download">● {labels.download}</span>
        <span class="legend-upload">● {labels.upload}</span>
      </figcaption>
    </figure>
  );
}
