import type { Project } from "./types.ts";

export interface DraftRepository {
  get(id: string): Promise<Project | null>;
  put(project: Project): Promise<void>;
  remove(id: string): Promise<void>;
}
type LegacyStorage = Pick<Storage, "getItem" | "removeItem">;

function indexedDrafts(): DraftRepository {
  let connection: Promise<IDBDatabase> | null = null;
  const open = () => {
    if (!connection) connection = new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open("yantai-recovery", 1);
      let blocked = false;
      request.onupgradeneeded = () => { request.result.createObjectStore("drafts", { keyPath: "id" }); };
      request.onblocked = () => { blocked = true; reject(Error("草稿缓存被其他窗口占用")); };
      request.onerror = () => reject(request.error);
      request.onsuccess = () => {
        if (blocked) { request.result.close(); return; }
        request.result.onversionchange = () => { request.result.close(); connection = null; };
        resolve(request.result);
      };
    }).catch(error => { connection = null; throw error; });
    return connection;
  };
  const transaction = async <T>(mode: IDBTransactionMode, work: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> => {
    const db = await open();
    return new Promise<T>((resolve, reject) => {
      const tx = db.transaction("drafts", mode), request = work(tx.objectStore("drafts"));
      tx.oncomplete = () => resolve(request.result);
      tx.onerror = () => reject(tx.error || request.error);
      tx.onabort = () => reject(tx.error || Error("草稿缓存写入未完成"));
    });
  };
  return {
    get: async id => (await transaction("readonly", store => store.get(id))) || null,
    put: async project => { await transaction("readwrite", store => store.put(project)); },
    remove: async id => { await transaction("readwrite", store => store.delete(id)); },
  };
}

// Queue cache writes and deletion in editing order. Cache failures never reject
// into the SQLite save path; legacy localStorage drafts remain recoverable.
export class DraftCache {
  private queue: Promise<unknown> = Promise.resolve();
  private repository: DraftRepository;
  private legacy: () => LegacyStorage;
  constructor(repository: DraftRepository, legacy: () => LegacyStorage) { this.repository = repository; this.legacy = legacy; }
  private enqueue<T>(work: () => Promise<T>): Promise<T> {
    const pending = this.queue.then(work);
    this.queue = pending.catch(() => {});
    return pending;
  }
  write(project: Project): Promise<boolean> {
    return this.enqueue(async () => {
      try {
        await this.repository.put(project);
        try { this.legacy().removeItem("draft:" + project.id); } catch { /* Preserve successful IDB write. */ }
        return true;
      } catch { return false; }
    });
  }
  read(id: string): Promise<Project | null> {
    return this.enqueue(async () => {
      try { const p = await this.repository.get(id); if (p?.id === id) return p; } catch { /* Try old cache. */ }
      try {
        const raw = this.legacy().getItem("draft:" + id);
        const p = raw ? JSON.parse(raw) : null;
        return p?.id === id && Array.isArray(p.chapters) ? p : null;
      } catch { return null; }
    });
  }
  remove(id: string): Promise<void> {
    return this.enqueue(async () => {
      try { await this.repository.remove(id); } catch { /* Database save already succeeded. */ }
      try { this.legacy().removeItem("draft:" + id); } catch { /* Database save already succeeded. */ }
    });
  }
}
export const draftCache = new DraftCache(indexedDrafts(), () => window.localStorage);
