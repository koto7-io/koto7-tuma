import { useEffect, useState } from "react";
import { cycleTheme, getStoredTheme, themeLabel, type Theme } from "../lib/theme";

const btnStyle: React.CSSProperties = {
  border: "1px solid var(--border)",
  background: "transparent",
  borderRadius: 6,
  padding: "8px 10px",
  cursor: "pointer",
  color: "var(--muted)",
  fontSize: 13,
  width: "100%",
  textAlign: "left",
};

export function ThemeToggle({ compact }: { compact?: boolean }) {
  const [theme, setThemeState] = useState<Theme>(() => getStoredTheme());

  useEffect(() => {
    setThemeState(getStoredTheme());
  }, []);

  function toggle() {
    setThemeState(cycleTheme());
  }

  if (compact) {
    return (
      <button type="button" onClick={toggle} style={{ ...btnStyle, width: "auto", position: "fixed", top: 16, right: 16 }}>
        {themeLabel(theme)}
      </button>
    );
  }

  return (
    <button type="button" onClick={toggle} style={btnStyle}>
      Theme · {themeLabel(theme)}
    </button>
  );
}
