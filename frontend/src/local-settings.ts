// Preferences must not interrupt editing when browser storage is full/disabled.
export const localSettings = {
  getItem(key: string): string | null { try { return window.localStorage.getItem(key); } catch { return null; } },
  setItem(key: string, value: string): void { try { window.localStorage.setItem(key, value); } catch { /* Best-effort preference only. */ } },
  removeItem(key: string): void { try { window.localStorage.removeItem(key); } catch { /* Best-effort cleanup. */ } },
};
