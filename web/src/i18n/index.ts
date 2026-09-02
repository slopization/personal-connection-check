export type Language = "ko" | "en";
export type Copy = {
  title: string;
  speed: string;
  stability: string;
  history: string;
  start: string;
  cancel: string;
  password: string;
  login: string;
  oidcLogin: string;
  loginFailed: string;
  pinging: string;
  downloading: string;
  uploading: string;
  measurementFailed: string;
  deleteAll: string;
  deleteConfirm: string;
  png: string;
  network: string;
  unsupported: string;
  webKitWarning: string;
  incompleteDownloadWarning: (count: number) => string;
  paused: string;
  methodology: string;
  chartAria: string;
  chartRtt: string;
  noSamples: string;
  liveSpeedChart: string;
  httpPing: string;
  downloadLabel: string;
  uploadLabel: string;
  jitter: string;
  range10m: string;
  range1h: string;
  rangeAll: string;
  mbps: (value: number | string) => string;
  milliseconds: (value: number | string) => string;
  saveData: (enabled: boolean) => string;
  speedResult: (download: number, upload: number) => string;
};
export const language = (
  v = typeof navigator === "undefined" ? "en" : navigator.language,
): Language => (v.toLowerCase().startsWith("ko") ? "ko" : "en");
export const text: Record<Language, Copy> = {
  en: {
    title: "Personal Connection Check",
    speed: "Speed test",
    stability: "Stability",
    history: "History",
    start: "Start test",
    cancel: "Cancel",
    password: "Shared password",
    login: "Sign in",
    oidcLogin: "Sign in with OIDC",
    loginFailed: "Sign-in failed",
    pinging: "Measuring ping…",
    downloading: "Downloading…",
    uploading: "Uploading…",
    measurementFailed: "Measurement failed",
    deleteAll: "Delete all",
    deleteConfirm: "Delete all local measurement history?",
    png: "Download PNG",
    network: "Network information",
    unsupported: "Not provided by this browser",
    webKitWarning:
      "Safari and WebKit-based browsers are not supported. The test remains available, but it may not complete; Chrome or Firefox is recommended.",
    incompleteDownloadWarning: (count) =>
      `${count} download stream${count === 1 ? "" : "s"} did not finish cleanly. The result uses bytes received before the deadline and may be lower than the actual speed.`,
    paused: "Measurement paused",
    methodology:
      "Measured to this server; results include the operational path.",
    chartAria: "Stability chart",
    chartRtt: "RTT",
    noSamples: "No samples",
    liveSpeedChart: "Live throughput chart",
    httpPing: "HTTP ping",
    downloadLabel: "Download",
    uploadLabel: "Upload",
    jitter: "jitter",
    range10m: "10m",
    range1h: "1h",
    rangeAll: "all",
    mbps: (value) => `${value} Mbps`,
    milliseconds: (value) => `${value} ms`,
    saveData: (enabled) => `saveData ${enabled}`,
    speedResult: (download, upload) =>
      `${download.toFixed(1)} Mbps / ${upload.toFixed(1)} Mbps`,
  },
  ko: {
    title: "개인 연결 점검",
    speed: "속도 측정",
    stability: "안정성",
    history: "이력",
    start: "측정 시작",
    cancel: "취소",
    password: "공유 비밀번호",
    login: "로그인",
    oidcLogin: "OIDC로 로그인",
    loginFailed: "로그인에 실패했습니다",
    pinging: "핑 측정 중…",
    downloading: "다운로드 중…",
    uploading: "업로드 중…",
    measurementFailed: "측정에 실패했습니다",
    deleteAll: "전체 삭제",
    deleteConfirm: "모든 로컬 측정 이력을 삭제할까요?",
    png: "PNG 저장",
    network: "네트워크 정보",
    unsupported: "브라우저에서 제공하지 않음",
    webKitWarning:
      "Safari 및 WebKit 계열 브라우저는 지원되지 않습니다. 측정은 시도할 수 있지만 완료되지 않을 수 있으므로 Chrome 또는 Firefox를 권장합니다.",
    incompleteDownloadWarning: (count) =>
      `다운로드 스트림 ${count}개가 정상적으로 끝나지 않았습니다. 제한 시간 전까지 받은 바이트로 계산되어 실제 속도보다 낮을 수 있습니다.`,
    paused: "측정 일시정지",
    methodology: "이 서버까지의 운영 경로를 측정한 결과입니다.",
    chartAria: "안정성 차트",
    chartRtt: "왕복 시간",
    noSamples: "샘플 없음",
    liveSpeedChart: "실시간 속도 그래프",
    httpPing: "HTTP 핑",
    downloadLabel: "다운로드",
    uploadLabel: "업로드",
    jitter: "지터",
    range10m: "10분",
    range1h: "1시간",
    rangeAll: "전체",
    mbps: (value) => `${value} Mbps`,
    milliseconds: (value) => `${value} ms`,
    saveData: (enabled) => `데이터 절약 ${enabled ? "사용" : "사용 안 함"}`,
    speedResult: (download, upload) =>
      `${download.toFixed(1)} Mbps / ${upload.toFixed(1)} Mbps`,
  },
};
