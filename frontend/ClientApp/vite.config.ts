import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'url'
import { defineConfig, loadEnv, type Plugin } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), 'DCS_')

  console.log('loaded Env:\n', env)

  const basePath = env.DCS_UI_PATH || '/ui/'

  const uiRedirectPlugin: Plugin = {
    name: 'ui-root-redirect',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const path = req.url?.split('?')[0] ?? ''
        if (path === '/' || path === '') {
          const q = req.url?.includes('?') ? req.url.slice(req.url.indexOf('?')) : ''
          res.writeHead(302, { Location: `${basePath}${q}` })
          res.end()
          return
        }
        next()
      })
    },
  }

  // Plugin to inject base href in dev mode
  const baseHrefPlugin: Plugin = {
    name: 'base-href-inject',
    transformIndexHtml: {
      order: 'pre',
      handler(html) {
        if (command === 'serve') {
          // In dev mode, replace the placeholder with the actual base path
          return html.replace('__DCS_UI_BASE_PATH__', basePath)
        }
        // In build mode, leave the placeholder for inject-config.sh to handle
        return html
      },
    },
  }

  return {
    // during build, use relative paths such that we respect <base href>
    base: command === 'build' ? './' : basePath,
    plugins: [uiRedirectPlugin, baseHrefPlugin, vue(), tailwindcss()],
    envPrefix: 'DCS_',
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src/', import.meta.url)),
        '@core': fileURLToPath(new URL('./src/core/', import.meta.url)),
        '@template-repository': fileURLToPath(new URL('./src/modules/template-repository/', import.meta.url)),
      },
    },
    server: {
      port: Number(env.DCS_FRONTEND_PORT) || 5173,
      proxy: {
        '/api': {
          target: env.DCS_API_TARGET || 'http://localhost:8991',
          changeOrigin: true,
        },
        // Proxy Hydra's public OIDC paths so the browser never needs a direct
        // Hydra address. Set DCS_HYDRA_TARGET to the Hydra public port
        // (e.g. http://localhost:4444 or the NodePort URL in a local cluster).
        '/oauth2': {
          target: env.DCS_HYDRA_TARGET || 'http://localhost:4444',
          changeOrigin: true,
        },
        '/.well-known': {
          target: env.DCS_HYDRA_TARGET || 'http://localhost:4444',
          changeOrigin: true,
        },
      },
    },
  }
})
