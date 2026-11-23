/**
 * Build web/chat into mobile/www for Capacitor.
 * Requires BAIZE_API_ORIGIN (or .env) when not using BAIZE_SERVER_URL live load.
 */
import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { config as loadEnv } from 'dotenv'

const mobileRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = resolve(mobileRoot, '..')
const chatRoot = join(repoRoot, 'web', 'chat')

loadEnv({ path: join(mobileRoot, '.env') })

const apiOrigin = (process.env.BAIZE_API_ORIGIN || process.env.VITE_API_ORIGIN || '').trim().replace(/\/$/, '')
const liveServer = (process.env.BAIZE_SERVER_URL || '').trim()

if (!apiOrigin && !liveServer) {
  console.warn(
    '[baize-mobile] Warning: neither BAIZE_API_ORIGIN nor BAIZE_SERVER_URL is set.\n' +
      '  Bundled www will call relative /v0 on the WebView origin (will fail).\n' +
      '  Copy mobile/.env.example → mobile/.env and set BAIZE_API_ORIGIN=https://your-baize-host',
  )
}

const env = {
  ...process.env,
  BAIZE_MOBILE: '1',
  VITE_API_ORIGIN: apiOrigin,
  VITE_ROUTER_BASENAME: '/',
}

console.log(`[baize-mobile] building web/chat → mobile/www (VITE_API_ORIGIN=${apiOrigin || '(empty)'})`)
const r = spawnSync('npm', ['run', 'build'], { cwd: chatRoot, env, stdio: 'inherit', shell: true })
if (r.status !== 0) process.exit(r.status ?? 1)

// Capacitor expects index at www/index.html; vite already wrote there when BAIZE_MOBILE=1.
const indexPath = join(mobileRoot, 'www', 'index.html')
if (!existsSync(indexPath)) {
  console.error(`[baize-mobile] missing ${indexPath}`)
  process.exit(1)
}

// Stamp a tiny marker for support / debugging (no secrets).
const stamp = {
  built_at: new Date().toISOString(),
  api_origin: apiOrigin || null,
  live_server: liveServer || null,
}
writeFileSync(join(mobileRoot, 'www', 'baize-mobile.json'), JSON.stringify(stamp, null, 2))
console.log('[baize-mobile] www ready')
