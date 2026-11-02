import {useCallback, useState} from "react";
import {localSettings} from "./local-settings";

// Save in the input handler, so closing the panel immediately cannot lose the
// final keystroke. Read the new scope directly instead of persisting old state
// in an effect when the user switches projects, chapters or writing tasks.
export default function useRememberedInput<T extends string>(key: string | null, fallback: T, allowed?: readonly T[]): [T, (value: T) => void] {
  const read = useCallback((): T => {
    const value = key ? localSettings.getItem(key) : null;
    return value !== null && (!allowed || allowed.includes(value as T)) ? value as T : fallback;
  }, [key, fallback, allowed]);
  const [state, setState] = useState<{key:string|null; value:T}>(()=>({key,value:read()}));
  const value = state.key === key ? state.value : read();
  const update = useCallback((next: T) => {
    if (key) localSettings.setItem(key, next);
    setState({key,value:next});
  }, [key]);
  return [value, update];
}
