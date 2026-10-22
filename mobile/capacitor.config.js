require('dotenv').config()

const live = (process.env.BAIZE_SERVER_URL || '').trim().replace(/\/$/, '')

/** @type {import('@capacitor/cli').CapacitorConfig} */
const config = {
  appId: 'com.rebornace.baize',
  appName: 'Baize',
  webDir: 'www',
  server: {
    androidScheme: 'https',
    cleartext: true,
    ...(live
      ? { url: live.endsWith('/ui') ? `${live}/` : `${live}/ui/` }
      : {}),
  },
  // Do not enable CapacitorHttp: it buffers responses and breaks SSE run streams.
  // Bundled www + BAIZE_API_ORIGIN needs Runtime CORS (BAIZE_CORS_ORIGINS); or use
  // BAIZE_SERVER_URL to load remote /ui same-origin (recommended for chat streaming).
}

module.exports = config
