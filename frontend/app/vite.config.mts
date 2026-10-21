import react from "@vitejs/plugin-react"
import path from "path"
import { defineConfig } from "vite"
import svgr from "vite-plugin-svgr"
import wails from "@wailsio/runtime/plugins/vite"

export default defineConfig({
  plugins: [
    {
      name: "desktop-dev-ready",
      configureServer(server) {
        let ready: Promise<void> | undefined
        server.middlewares.use((req, res, next) => {
          if (req.url !== "/__dev_ready") return next()
          // HTML alone is ready before the initial module crawl and dependency optimisation.
          ready ??= (async () => {
            await server.environments.client.warmupRequest("/src/index.tsx")
            await server.environments.client.waitForRequestsIdle()
          })()
          ready.then(() => res.end("ready"), next)
        })
      },
    },
    react(),
    wails("../bindings"),
    svgr({
      include: "**/*.svg",
      svgrOptions: {
        plugins: ["@svgr/plugin-svgo", "@svgr/plugin-jsx"],
        exportType: "default",
      },
    }),
  ],
  base: "./",
  // Concurrent dev servers must not overwrite each other's optimised dependencies.
  cacheDir: `node_modules/.vite-${process.env.WAILS_VITE_PORT || "default"}`,
  optimizeDeps: {
    include: ["@emotion/react/jsx-runtime"],
  },
  build: {
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, "index.html"),
      },
    },
    minify: false,
    sourcemap: true,
    outDir: "../dist",
    emptyOutDir: true,
  },
  publicDir: "public",
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  resolve: {
    alias: {
      react: path.resolve("../node_modules/react"),
    },
    dedupe: ["react", "react-dom"],
  },
})
