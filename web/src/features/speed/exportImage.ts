import type { SpeedResult } from "../../storage/database";
export type ExportInput = {
  result: SpeedResult;
  title: string;
  methodology: string;
};
export async function exportPNG(input: ExportInput): Promise<Blob> {
  const c = document.createElement("canvas");
  c.width = 1200;
  c.height = 630;
  const x = c.getContext("2d");
  if (!x) throw new Error("canvas unavailable");
  x.fillStyle = "#101827";
  x.fillRect(0, 0, c.width, c.height);
  x.fillStyle = "#fff";
  x.font = "bold 48px sans-serif";
  x.fillText(input.title, 60, 90);
  x.font = "32px sans-serif";
  const r = input.result;
  const lines = [
    `Download: ${r.download.toFixed(1)} Mbps`,
    `Upload: ${r.upload.toFixed(1)} Mbps`,
    `Ping: ${r.ping.toFixed(0)} ms`,
    new Date(r.at).toLocaleString(),
    r.server ?? "",
    r.ip ?? "",
    [r.isp, r.city].filter(Boolean).join(" · "),
    input.methodology,
  ];
  lines.filter(Boolean).forEach((v, i) => x.fillText(v, 60, 160 + i * 52));
  return new Promise((ok, bad) =>
    c.toBlob(
      (b) => (b ? ok(b) : bad(new Error("PNG encoding failed"))),
      "image/png",
    ),
  );
}
