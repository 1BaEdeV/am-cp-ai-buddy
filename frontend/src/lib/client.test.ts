import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})
it.each([undefined, 'demo', 'api'])('выбирает нужный источник данных (%s)', async (mode) => {
  vi.resetModules()
  vi.stubEnv('VITE_DATA_SOURCE', mode)
  const { client, isDemo } = await import('./client')
  const { demoClient } = await import('./demo')
  const { apiClient } = await import('./api')
  expect(isDemo).toBe(mode !== 'api')
  expect(client).toBe(mode === 'api' ? apiClient : demoClient)
})
