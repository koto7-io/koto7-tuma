import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/e": "http://localhost:8080",
      "/sink": "http://localhost:8080",
      "/playground/r": "http://localhost:8080",
    },
  },
});
