import { useEffect, useState } from "react";
import { cycleTheme, getStoredTheme, themeLabel, type Theme } from "../lib/theme";

export function ThemeToggle({ compact }: { compact?: boolean }) {
  const [theme, setThemeState] = useState<Theme>(() => getStoredTheme());

  useEffect(() => {
    setThemeState(getStoredTheme());
  }, []);

  function toggle() {
    setThemeState(cycleTheme());
  }

  const className = compact ? "tuma-theme-toggle tuma-theme-toggle--compact" : "tuma-theme-toggle";

  return (
    <button type="button" onClick={toggle} className={className}>
      {compact ? themeLabel(theme) : `Theme · ${themeLabel(theme)}`}
    </button>
  );
}
