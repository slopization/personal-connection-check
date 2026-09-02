import { jitter, type Sample } from "./metrics";
export type MonitorOptions = {
  url: string;
  onSample: (s: Sample, jitter: number) => void;
  onPause: () => void;
  WebSocketImpl?: typeof WebSocket;
  visible?: () => boolean;
};
export class StabilityMonitor {
  private ws?: WebSocket;
  private timer?: number;
  private stopped = false;
  private samples: Sample[] = [];
  private retry = 0;
  private started = 0;
  constructor(private o: MonitorOptions) {}
  start() {
    this.stopped = false;
    this.connect();
    document.addEventListener("visibilitychange", this.visibility);
  }
  stop() {
    this.stopped = true;
    this.clear();
    this.ws?.close();
    document.removeEventListener("visibilitychange", this.visibility);
  }
  private visibility = () => {
    if (!this.o.visible?.() && document.visibilityState !== "visible") {
      this.clear();
      this.ws?.close();
      this.o.onPause();
    } else this.connect();
  };
  private clear() {
    if (this.timer) window.clearInterval(this.timer);
    this.timer = undefined;
  }
  private connect() {
    if (
      this.stopped ||
      (!this.o.visible?.() && document.visibilityState !== "visible")
    )
      return;
    this.ws?.close();
    const W = this.o.WebSocketImpl ?? WebSocket;
    this.ws = new W(this.o.url);
    this.ws.onopen = () => {
      this.retry = 0;
      this.tick();
      this.timer = window.setInterval(() => this.tick(), 1000);
    };
    this.ws.onmessage = () => {
      const s = { at: Date.now(), rtt: performance.now() - this.started };
      this.samples.push(s);
      this.o.onSample(s, jitter(this.samples));
    };
    this.ws.onclose = () => {
      this.clear();
      if (!this.stopped) {
        const delay = Math.min(10000, 250 * 2 ** this.retry++);
        window.setTimeout(() => this.connect(), delay);
      }
    };
  }
  private tick() {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.started = performance.now();
      this.ws.send(String(Date.now()));
    }
  }
}
