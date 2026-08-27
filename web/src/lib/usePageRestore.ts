import { useEffect } from "react";

/** Run on mount and when restored from bfcache (back/forward). React effects skip bfcache restore. */
export function usePageRestore(effect: () => void | (() => void), deps: readonly unknown[]) {
  useEffect(() => {
    const cleanup = effect();
    const onShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return;
      if (typeof cleanup === "function") cleanup();
      effect();
    };
    window.addEventListener("pageshow", onShow);
    return () => {
      window.removeEventListener("pageshow", onShow);
      if (typeof cleanup === "function") cleanup();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
