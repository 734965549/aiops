import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ArcoResolver } from 'unplugin-vue-components/resolvers'

// 使用 realpath 统一 root，避免 Windows 下 subst/网盘映射/中文路径导致
// path.relative 失败，进而触发 Rollup「emitted chunks must be strings」构建错误。
// 见 https://github.com/vitejs/vite/issues/10802
const projectRoot = fs.realpathSync.native(
  path.dirname(fileURLToPath(import.meta.url))
)

// 后端 API 默认 8080，前端 dev server 默认 5173；通过 vite 反向代理 /api。
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, projectRoot, '')
  const apiTarget = env.VITE_API_BASE || 'http://127.0.0.1:8080'

  return {
    root: projectRoot,
    plugins: [
      vue(),
      AutoImport({ resolvers: [ArcoResolver()] }),
      Components({
        resolvers: [
          ArcoResolver({ sideEffect: true, resolveIcons: true })
        ]
      })
    ],
    resolve: {
      // 与 root 配合，避免 junction/网盘同步目录下 cwd 与 realpath 不一致。
      preserveSymlinks: true,
      alias: {
        '@': path.resolve(projectRoot, 'src')
      }
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true
        },
        '/healthz': { target: apiTarget, changeOrigin: true },
        '/readyz': { target: apiTarget, changeOrigin: true },
        '/version': { target: apiTarget, changeOrigin: true }
      }
    },
    build: {
      outDir: 'dist',
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes('node_modules')) {
              return
            }
            if (id.includes('@arco-design/web-vue')) {
              return 'arco'
            }
            if (
              id.includes('/vue/') ||
              id.includes('/vue-router/') ||
              id.includes('/pinia/') ||
              id.includes('\\vue\\') ||
              id.includes('\\vue-router\\') ||
              id.includes('\\pinia\\')
            ) {
              return 'vue-vendor'
            }
            if (id.includes('axios')) {
              return 'axios'
            }
          }
        }
      }
    }
  }
})
