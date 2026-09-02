export type SpeedResult = {
  at: number;
  download: number;
  upload: number;
  ping: number;
  server?: string;
  ip?: string;
  isp?: string;
  city?: string;
};
export type StabilitySession = {
  at: number;
  samples: { at: number; rtt: number }[];
  pauses: number[];
};
type StoreName = "speed" | "stability";
type Stored = SpeedResult | StabilitySession;
type Row = { store: StoreName; value: Stored; size: number };
const DB = "pcc-history",
  VERSION = 2;
const open = () =>
  new Promise<IDBDatabase>((ok, bad) => {
    const r = indexedDB.open(DB, VERSION);
    r.onupgradeneeded = () => {
      const d = r.result;
      if (!d.objectStoreNames.contains("speed"))
        d.createObjectStore("speed", { keyPath: "at" });
      if (!d.objectStoreNames.contains("stability"))
        d.createObjectStore("stability", { keyPath: "at" });
    };
    r.onsuccess = () => ok(r.result);
    r.onerror = () => bad(r.error);
  });
export const budget = async () =>
  Math.min(
    20 * 1024 * 1024,
    ((await navigator.storage?.estimate?.())?.quota ?? 400 * 1024 * 1024) *
      0.05,
  );
const sizeOf = (value: unknown) =>
  new TextEncoder().encode(JSON.stringify(value)).byteLength;
const request = <T>(r: IDBRequest<T>) =>
  new Promise<T>((ok, bad) => {
    r.onsuccess = () => ok(r.result);
    r.onerror = () => bad(r.error);
  });
async function rows(db: IDBDatabase): Promise<Row[]> {
  const [speed, stability] = await Promise.all(
    (["speed", "stability"] as const).map(async (store) =>
      (await request(db.transaction(store).objectStore(store).getAll())).map(
        (value) => ({ store, value, size: sizeOf(value) }),
      ),
    ),
  );
  return [...speed, ...stability];
}
async function deleteRows(db: IDBDatabase, evicted: Row[]) {
  if (!evicted.length) return;
  const tx = db.transaction(["speed", "stability"], "readwrite");
  for (const row of evicted) tx.objectStore(row.store).delete(row.value.at);
  await new Promise<void>((ok, bad) => {
    tx.oncomplete = () => ok();
    tx.onerror = () => bad(tx.error);
    tx.onabort = () => bad(tx.error);
  });
}
async function evictToBudget(db: IDBDatabase, store: StoreName, value: Stored) {
  const oldRows = await rows(db);
  const incoming = sizeOf(value);
  const retained = oldRows.filter(
    (row) => row.store !== store || row.value.at !== value.at,
  );
  let projected =
    retained.reduce((total, row) => total + row.size, 0) + incoming;
  const limit = await budget();
  const evicted: Row[] = [];
  for (const row of retained.sort((a, b) => a.value.at - b.value.at)) {
    if (projected <= limit) break;
    projected -= row.size;
    evicted.push(row);
  }
  await deleteRows(db, evicted);
}
async function evictOldest(db: IDBDatabase) {
  const [oldest] = (await rows(db)).sort((a, b) => a.value.at - b.value.at);
  if (oldest) await deleteRows(db, [oldest]);
}
async function put(store: StoreName, value: Stored) {
  const db = await open();
  const write = () =>
    request(
      db.transaction(store, "readwrite").objectStore(store).put(value),
    ).then(() => undefined);
  try {
    await evictToBudget(db, store, value);
    try {
      await write();
    } catch {
      await evictOldest(db);
      await write();
    }
  } finally {
    db.close();
  }
}
export const save = (x: SpeedResult) => put("speed", x);
export const saveStability = (x: StabilitySession) => put("stability", x);
async function listStore<T extends Stored>(store: StoreName): Promise<T[]> {
  const db = await open();
  try {
    const values = await request(
      db.transaction(store).objectStore(store).getAll(),
    );
    return (values as T[]).sort((a, b) => b.at - a.at);
  } finally {
    db.close();
  }
}
export const list = () => listStore<SpeedResult>("speed");
export const listStability = () => listStore<StabilitySession>("stability");
export async function latestStability() {
  return (await listStability())[0];
}
export const clear = () =>
  new Promise<void>((ok, bad) => {
    const r = indexedDB.deleteDatabase(DB);
    r.onsuccess = () => ok();
    r.onerror = () => bad(r.error);
  });
