import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { initTheme } from "./lib/theme";
import "./styles/tokens.css";
import "./styles/components.css";
import "./styles/console.css";

initTheme();

// bfcache can restore #root before React repaints; reload instead of showing a blank shell.
window.addEventListener("pageshow", (event) => {
  if (event.persisted && !document.getElementById("root")?.childElementCount) {
    window.location.reload();
  }
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>
);
