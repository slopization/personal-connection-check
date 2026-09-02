import { useEffect, useRef, useState } from "preact/hooks";
import { language, text } from "../i18n";
import { isUnsupportedWebKit } from "../browser";
import {
  clear,
  latestStability,
  list,
  save,
  saveStability,
  type SpeedResult,
} from "../storage/database";
import { exportPNG } from "../features/speed/exportImage";
import { runAdaptive } from "../features/speed/engine";
import { SpeedChart } from "../features/speed/SpeedChart";
import {
  createBrowserTransport,
  type LiveSpeedSample,
} from "../features/speed/protocol";
import { StabilityMonitor } from "../features/stability/monitor";
import { jitter, type Sample } from "../features/stability/metrics";
import {
  StabilityChart,
  type ChartRange,
} from "../features/stability/StabilityChart";
const t = text[language()];
type Info = { ip: string; city?: string; isp?: string; country?: string };
type AuthConfig = { password: boolean; oidc: boolean; footerMessage?: string };
type Net = {
  type?: string;
  effectiveType?: string;
  downlink?: number;
  rtt?: number;
  saveData?: boolean;
};
const stabilityPingURL = () =>
  `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/ping`;
export async function warmPing(
  signal: AbortSignal,
  timeoutMs = 10_000,
  fetcher: typeof fetch = fetch,
  now = () => performance.now(),
) {
  const ctl = new AbortController();
  let timedOut = false;
  const abort = () => ctl.abort();
  if (signal.aborted) abort();
  else signal.addEventListener("abort", abort, { once: true });
  const timeout = window.setTimeout(() => {
    timedOut = true;
    ctl.abort();
  }, timeoutMs);
  const values: number[] = [];
  try {
    for (let index = 0; index < 8; index++) {
      const started = now();
      const response = await fetcher(`/healthz?ping=${index}`, {
        signal: ctl.signal,
        cache: "no-store",
      });
      if (!response.ok) throw new Error("Ping failed");
      values.push(now() - started);
    }
  } catch (error) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    if (timedOut) throw new Error("Ping timed out");
    throw error;
  } finally {
    window.clearTimeout(timeout);
    signal.removeEventListener("abort", abort);
  }
  const ranked = values.slice(2).sort((a, b) => a - b);
  return ranked[Math.floor(ranked.length / 2)];
}
export async function closeRun(
  id: string,
  fetcher: typeof fetch = fetch,
): Promise<boolean> {
  try {
    const response = await fetcher(`/api/test-runs/${encodeURIComponent(id)}`, {
      method: "DELETE",
      keepalive: true,
    });
    return response.ok || response.status === 404;
  } catch {
    // The server-side TTL remains the fail-safe when the browser is offline.
    return false;
  }
}
export function downloadBlob(
  blob: Blob,
  doc: Document = document,
  createObjectURL: (blob: Blob) => string = URL.createObjectURL.bind(URL),
  revokeObjectURL: (url: string) => void = URL.revokeObjectURL.bind(URL),
): void {
  const href = createObjectURL(blob);
  const anchor = doc.createElement("a");
  anchor.href = href;
  anchor.download = "connection-check.png";
  doc.body.append(anchor);
  try {
    anchor.click();
  } finally {
    window.setTimeout(() => {
      anchor.remove();
      revokeObjectURL(href);
    }, 1000);
  }
}
export function warnIncompleteDownload(
  incompleteStreams: number,
  logger: (
    message: string,
    detail: { incompleteStreams: number },
  ) => void = console.warn,
): boolean {
  if (incompleteStreams < 1) return false;
  logger("Download measurement used partial stream snapshots", {
    incompleteStreams,
  });
  return true;
}
export function FooterMessage({ message }: { message: string }) {
  if (!message) return null;
  return (
    <footer class="admin-message">
      {message
        .split(/(https?:\/\/[^\s<]+)/g)
        .map((part) =>
          /^https?:\/\//.test(part) ? <a href={part}>{part}</a> : part,
        )}
    </footer>
  );
}
export function IncompleteDownloadWarning({ count }: { count: number }) {
  if (count < 1) return null;
  return (
    <p class="measurement-warning" role="alert">
      {t.incompleteDownloadWarning(count)}
    </p>
  );
}
export function App() {
  const unsupportedWebKit = isUnsupportedWebKit();
  const [password, setPassword] = useState("");
  const [logged, setLogged] = useState(false);
  const [status, setStatus] = useState("");
  const [tab, setTab] = useState<"speed" | "stability" | "history">("speed");
  const [history, setHistory] = useState<SpeedResult[]>([]);
  const [result, setResult] = useState<SpeedResult>();
  const [liveSpeed, setLiveSpeed] = useState<LiveSpeedSample[]>([]);
  const [livePing, setLivePing] = useState<number>();
  const [running, setRunning] = useState(false);
  const [pngBlob, setPNGBlob] = useState<Blob>();
  const [info, setInfo] = useState<Info>();
  const [samples, setSamples] = useState<Sample[]>([]);
  const [pauses, setPauses] = useState<number[]>([]);
  const [monitoring, setMonitoring] = useState(false);
  const [chartRange, setChartRange] = useState<ChartRange>("10m");
  const [footerMessage, setFooterMessage] = useState("");
  const [authConfig, setAuthConfig] = useState<AuthConfig>();
  const runCtl = useRef<AbortController>();
  const monitor = useRef<StabilityMonitor>();
  const network = (navigator as Navigator & { connection?: Net }).connection;
  useEffect(
    () => () => {
      runCtl.current?.abort();
      monitor.current?.stop();
    },
    [],
  );
  async function hydrateStability() {
    const session = await latestStability();
    if (session) {
      setSamples(session.samples);
      setPauses(session.pauses);
    }
  }
  useEffect(() => {
    void hydrateStability();
  }, []);
  useEffect(() => {
    let active = true;
    void fetch("/api/auth/config")
      .then((response) => response.json() as Promise<AuthConfig>)
      .then((config) => {
        if (active) {
          setAuthConfig(config);
          if (typeof config.footerMessage === "string")
            setFooterMessage(config.footerMessage);
          if (config.oidc) {
            void fetch("/api/network-info")
              .then((response) => {
                if (!response.ok) throw new Error("no active session");
                return response.json() as Promise<Info>;
              })
              .then((restoredInfo) => {
                if (active) {
                  setInfo(restoredInfo);
                  setLogged(true);
                }
              })
              .catch(() => undefined);
          }
        }
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, []);
  async function login() {
    try {
      const r = await fetch("/api/login/password", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ password }),
      });
      setLogged(r.ok);
      setStatus(r.ok ? "" : t.loginFailed);
      if (r.ok) setInfo(await fetch("/api/network-info").then((x) => x.json()));
    } catch {
      setStatus(t.loginFailed);
    }
  }
  async function measure() {
    if (monitoring) return;
    setPNGBlob(undefined);
    setResult(undefined);
    setLiveSpeed([]);
    setLivePing(undefined);
    const ctl = new AbortController();
    let runID: string | undefined;
    runCtl.current = ctl;
    setRunning(true);
    try {
      setStatus(t.pinging);
      const ping = await warmPing(ctl.signal);
      setLivePing(ping);
      const created = await fetch("/api/test-runs", {
        method: "POST",
        signal: ctl.signal,
      });
      if (!created.ok) throw new Error("run failed");
      const run = (await created.json()) as { id: string };
      runID = run.id;
      setStatus(t.downloading);
      const value = await runAdaptive(
        createBrowserTransport(
          run.id,
          fetch,
          () => performance.now(),
          (sample) => {
            setLiveSpeed((current) => [...current.slice(-159), sample]);
            setStatus(
              sample.direction === "download" ? t.downloading : t.uploading,
            );
          },
        ),
        ctl.signal,
      );
      warnIncompleteDownload(value.incompleteDownloadStreams);
      if (await closeRun(run.id)) runID = undefined;
      const stored: SpeedResult = {
        at: Date.now(),
        download: value.downloadMbps,
        upload: value.uploadMbps,
        ping,
        ip: info?.ip,
        isp: info?.isp,
        city: info?.city,
        incompleteDownloadStreams: value.incompleteDownloadStreams,
      };
      await save(stored);
      let preparedPNG: Blob | undefined;
      try {
        preparedPNG = await exportPNG({
          result: stored,
          title: t.title,
          methodology: t.methodology,
        });
      } catch {
        // Measurement results remain usable if this browser cannot encode PNG.
      }
      setResult(stored);
      setPNGBlob(preparedPNG);
      setStatus(t.speedResult(stored.download, stored.upload));
    } catch (e) {
      if ((e as DOMException).name !== "AbortError")
        setStatus(t.measurementFailed);
    } finally {
      if (runID) await closeRun(runID);
      runCtl.current = undefined;
      setRunning(false);
    }
  }
  function stopMonitoring() {
    monitor.current?.stop();
    monitor.current = undefined;
    if (monitoring) void saveStability({ at: Date.now(), samples, pauses });
    setMonitoring(false);
  }
  function startMonitoring() {
    if (runCtl.current) return;
    const m = new StabilityMonitor({
      url: stabilityPingURL(),
      onSample: (s) => setSamples((x) => [...x, s]),
      onPause: () => setPauses((x) => [...x, Date.now()]),
    });
    monitor.current = m;
    setMonitoring(true);
    m.start();
  }
  async function show() {
    setHistory(await list());
  }

  if (!logged)
    return (
      <main>
        <h1>{t.title}</h1>
        {unsupportedWebKit && (
          <p class="browser-warning" role="alert">
            {t.webKitWarning}
          </p>
        )}
        {authConfig?.password && (
          <>
            <label>
              {t.password}
              <input
                aria-label={t.password}
                type="password"
                value={password}
                onInput={(e) =>
                  setPassword((e.target as HTMLInputElement).value)
                }
              />
            </label>
            <button onClick={login}>{t.login}</button>
          </>
        )}
        {authConfig?.oidc && <a href="/api/auth/oidc/begin">{t.oidcLogin}</a>}
        <p role="alert">{status}</p>
        <FooterMessage message={footerMessage} />
      </main>
    );
  return (
    <main>
      <header>
        <h1>{t.title}</h1>
        <nav aria-label={t.title}>
          {(["speed", "stability", "history"] as const).map((x) => (
            <button
              aria-pressed={tab === x}
              onClick={() => {
                if (x !== "stability") stopMonitoring();
                setTab(x);
                if (x === "history") void show();
                if (x === "stability") void hydrateStability();
              }}
            >
              {t[x]}
            </button>
          ))}
        </nav>
      </header>
      {unsupportedWebKit && (
        <p class="browser-warning" role="alert">
          {t.webKitWarning}
        </p>
      )}
      {tab === "speed" && (
        <section>
          <button
            disabled={monitoring || running}
            onClick={() => void measure()}
          >
            {t.start}
          </button>
          {running && (
            <button onClick={() => runCtl.current?.abort()}>{t.cancel}</button>
          )}
          <p aria-live="polite">{status}</p>
          {(running || liveSpeed.length > 0 || livePing !== undefined) && (
            <div class="live-measurement">
              <div class="live-metrics" aria-live="polite">
                <div>
                  <span>
                    {liveSpeed.at(-1)?.direction === "upload"
                      ? t.uploadLabel
                      : t.downloadLabel}
                  </span>
                  <strong>{liveSpeed.at(-1)?.mbps.toFixed(1) ?? "–"}</strong>
                  <small>Mbps</small>
                </div>
                <div>
                  <span>{t.httpPing}</span>
                  <strong>{livePing?.toFixed(1) ?? "–"}</strong>
                  <small>ms</small>
                </div>
              </div>
              <SpeedChart
                samples={liveSpeed}
                labels={{
                  aria: t.liveSpeedChart,
                  download: t.downloadLabel,
                  upload: t.uploadLabel,
                  empty: t.noSamples,
                }}
              />
            </div>
          )}
          {info && (
            <aside>
              <h2>{t.network}</h2>
              <p>
                {info.ip}{" "}
                {[info.city, info.country, info.isp]
                  .filter(Boolean)
                  .join(" · ")}
              </p>
              {network && (
                <p>
                  {[
                    network.type,
                    network.effectiveType,
                    network.downlink && t.mbps(network.downlink),
                    network.rtt && t.milliseconds(network.rtt),
                    network.saveData !== undefined &&
                      t.saveData(network.saveData),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
              )}
            </aside>
          )}
          {result && pngBlob && (
            <button onClick={() => downloadBlob(pngBlob)}>{t.png}</button>
          )}
          <IncompleteDownloadWarning
            count={result?.incompleteDownloadStreams ?? 0}
          />
        </section>
      )}
      {tab === "stability" && (
        <section>
          <button onClick={monitoring ? stopMonitoring : startMonitoring}>
            {monitoring ? t.cancel : t.start}
          </button>
          <p aria-live="polite">
            {monitoring
              ? `${t.milliseconds(samples.at(-1)?.rtt?.toFixed(1) ?? "–")} · ${t.jitter} ${t.milliseconds(jitter(samples).toFixed(1))}`
              : t.paused}
          </p>
          <div>
            {(["10m", "1h", "all"] as ChartRange[]).map((x) => (
              <button
                aria-pressed={chartRange === x}
                onClick={() => setChartRange(x)}
              >
                {{ "10m": t.range10m, "1h": t.range1h, all: t.rangeAll }[x]}
              </button>
            ))}
          </div>
          <StabilityChart
            samples={samples}
            selected={chartRange}
            labels={{ aria: t.chartAria, rtt: t.chartRtt, empty: t.noSamples }}
          />
          {pauses.map((p) => (
            <time dateTime={new Date(p).toISOString()}>{t.paused}</time>
          ))}
        </section>
      )}
      {tab === "history" && (
        <section>
          <button
            onClick={async () => {
              if (confirm(t.deleteConfirm)) {
                await clear();
                setHistory([]);
              }
            }}
          >
            {t.deleteAll}
          </button>
          <ul>
            {history.map((x) => (
              <li>
                {new Date(x.at).toLocaleString()}:{" "}
                {t.mbps(x.download.toFixed(1))}/ {t.mbps(x.upload.toFixed(1))}
              </li>
            ))}
          </ul>
        </section>
      )}
      <FooterMessage message={footerMessage} />
    </main>
  );
}
