import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient as client } from './api'

afterEach(() => vi.unstubAllGlobals())
describe('HTTP-контракт frontend', () => {
  it('подтверждает один черновик в контексте пользователя без сервисного токена', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'request-1' }), { status: 201 }))
    vi.stubGlobal('fetch', fetch)
    await client.confirm('intern-alina', 'draft/1')
    const [path, options] = fetch.mock.calls[0]
    expect(path).toBe('/api/v1/access-requests/drafts/draft%2F1/confirm')
    expect(options.headers).toEqual({ 'X-Demo-Employee-Id': 'intern-alina', 'Content-Type': 'application/json' })
    expect(JSON.parse(options.body)).toEqual({ confirmed: true })
  })
  it('передаёт фильтр и пагинацию, а для чата создаёт ключ повтора', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(new Response('{}')).mockResolvedValueOnce(new Response('{}'))
    vi.stubGlobal('fetch', fetch)
    await client.requests('intern-alina', 'completed', 20)
    expect(fetch.mock.calls[0][0]).toBe('/api/v1/access-requests?group=completed&limit=20&offset=20')
    await client.sendMessage('intern-alina', 'working-accesses', 'Запроси GitLab', 'gitlab-search-api')
    const options = fetch.mock.calls[1][1]
    expect(JSON.parse(options.body)).toEqual({ text: 'Запроси GitLab' })
    expect(options.headers['Idempotency-Key']).toBeTruthy()
    expect(options.headers.Authorization).toBeUndefined()
  })
  it('сохраняет понятную ошибку API, обрабатывает HTML-ошибку и сбой сети', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ code: 'draft_changed', message: 'Подготовь новый черновик' }), { status: 409 })).mockResolvedValueOnce(new Response('404 page not found', { status: 404 })).mockRejectedValueOnce(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetch)
    await expect(client.confirm('intern', 'draft')).rejects.toMatchObject({ message: 'Подготовь новый черновик', status: 409 })
    await expect(client.plan('intern')).rejects.toMatchObject({ status: 404 })
    await expect(client.login('intern')).rejects.toMatchObject({ code: 'network_error' })
  })
})
