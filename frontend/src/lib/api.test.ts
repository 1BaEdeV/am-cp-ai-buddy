import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient as client } from './api'

afterEach(() => vi.unstubAllGlobals())
describe('HTTP-контракт frontend', () => {
  it('отправляет вход, регистрацию и возвращает вложенные данные карточек', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: 'registration_required' })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ employeeId: 'intern' })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ workingAccesses: { resources: [] } })))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ teamIntro: { description: 'Команда' } })),
      )
    vi.stubGlobal('fetch', fetch)
    expect(await client.login('intern')).toEqual({ status: 'registration_required' })
    const input = { positionId: 'position', mentorId: 'mentor', internshipStartDate: '2026-11-16' }
    expect(await client.register('intern', input)).toEqual({ employeeId: 'intern' })
    expect(fetch.mock.calls[0][1].headers).toEqual({ 'Content-Type': 'application/json' })
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ identifier: 'intern' })
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual(input)
    expect(await client.accesses('intern')).toEqual({ resources: [] })
    expect(await client.intro('intern')).toEqual({ description: 'Команда' })
  })
  it('читает карточку, план и чат; отправляет решение, повтор выдачи и завершение знакомства', async () => {
    const fetch = vi.fn().mockImplementation(async () => new Response('{}'))
    vi.stubGlobal('fetch', fetch)
    await client.request('manager', 'request/1')
    await client.decide('manager', 'request/1', 'reject', 'Уточни цель')
    await client.retry('manager', 'request/1')
    await client.plan('intern')
    await client.chat('intern', 'team-intro')
    await client.completeIntro('intern')
    await client.requests('intern')
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/access-requests/request%2F1',
      '/api/v1/access-requests/request%2F1/decision',
      '/api/v1/access-requests/request%2F1/retry',
      '/api/v1/onboarding',
      '/api/v1/onboarding/tasks/team-intro/chat',
      '/api/v1/onboarding/tasks/team-intro/complete',
      '/api/v1/access-requests?group=all&limit=20&offset=0',
    ])
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({
      decision: 'reject',
      reason: 'Уточни цель',
    })
    expect(JSON.parse(fetch.mock.calls[2][1].body)).toEqual({})
    expect(fetch.mock.calls[0][1].method).toBe('GET')
  })
  it('принимает ответ без содержимого и отклоняет повреждённый успешный ответ', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(new Response(null, { status: 204 }))
        .mockResolvedValueOnce(new Response('<html>Proxy</html>'))
        .mockResolvedValueOnce(new Response('null'))
        .mockResolvedValueOnce(new Response('{}', { status: 503 })),
    )
    await expect(client.completeIntro('intern')).resolves.toBeUndefined()
    await expect(client.plan('intern')).rejects.toMatchObject({
      status: 502,
      code: 'invalid_response',
    })
    await expect(client.chat('intern', 'team-intro')).rejects.toMatchObject({
      code: 'invalid_response',
    })
    await expect(client.requests('intern')).rejects.toMatchObject({
      status: 503,
      code: 'http_error',
      message: 'Не удалось выполнить действие. Повтори попытку.',
    })
  })
  it('подтверждает один черновик в контексте пользователя без сервисного токена', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: 'request-1' }), { status: 201 }))
    vi.stubGlobal('fetch', fetch)
    await client.confirm('intern-alina', 'draft/1')
    const [path, options] = fetch.mock.calls[0]
    expect(path).toBe('/api/v1/access-requests/drafts/draft%2F1/confirm')
    expect(options.headers).toEqual({
      'X-Demo-Employee-Id': 'intern-alina',
      'Content-Type': 'application/json',
    })
    expect(JSON.parse(options.body)).toEqual({ confirmed: true })
  })
  it('передаёт фильтр и пагинацию, а для чата создаёт ключ повтора', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(new Response('{}'))
      .mockResolvedValueOnce(new Response('{}'))
    vi.stubGlobal('fetch', fetch)
    await client.requests('intern-alina', 'completed', 20)
    expect(fetch.mock.calls[0][0]).toBe(
      '/api/v1/access-requests?group=completed&limit=20&offset=20',
    )
    await client.sendMessage(
      'intern-alina',
      'working-accesses',
      'Запроси GitLab',
      'gitlab-search-api',
    )
    const options = fetch.mock.calls[1][1]
    expect(JSON.parse(options.body)).toEqual({ text: 'Запроси GitLab' })
    expect(options.headers['Idempotency-Key']).toBeTruthy()
    expect(options.headers.Authorization).toBeUndefined()
  })
  it('приводит ответ чата Go к формату интерфейса', async () => {
    const draft = { id: 'draft-1' }
    const body = {
      taskId: 'working-accesses',
      messages: [{ id: 'message-1', role: 'assistant', text: 'Готово', accessDraft: draft }],
      latestRun: { id: 'run-1', status: 'failed', error: { code: 'failed', message: 'Повтори' } },
    }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body))))
    const chat = await client.chat('intern-alina', 'working-accesses')
    expect(chat.messages[0].draft).toEqual(draft)
    expect(chat.latestRun?.error).toBe('Повтори')
  })
  it('сохраняет понятную ошибку API, обрабатывает HTML-ошибку и сбой сети', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: 'draft_changed', message: 'Подготовь новый черновик' }),
          { status: 409 },
        ),
      )
      .mockResolvedValueOnce(new Response('404 page not found', { status: 404 }))
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetch)
    await expect(client.confirm('intern', 'draft')).rejects.toMatchObject({
      message: 'Подготовь новый черновик',
      status: 409,
    })
    await expect(client.plan('intern')).rejects.toMatchObject({ status: 404 })
    await expect(client.login('intern')).rejects.toMatchObject({ code: 'network_error' })
  })
})
