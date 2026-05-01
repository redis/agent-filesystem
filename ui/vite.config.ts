import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { tanstackRouter } from "@tanstack/router-plugin/vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react(),
  ],
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: "react-runtime",
              test: /node_modules[\\/](?:react|react-dom)[\\/]/,
              priority: 40,
            },
            {
              name: "tanstack",
              test: /node_modules[\\/]@tanstack[\\/]/,
              priority: 30,
            },
          ],
        },
      },
    },
  },
});
