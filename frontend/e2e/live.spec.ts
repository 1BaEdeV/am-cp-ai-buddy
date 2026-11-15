import { expect, test } from '@playwright/test'

// Только для отдельной тестовой БД. Токен используется процессом теста,
// в страницу не передаётся. Фикстура подменяет лишь ещё не подключённый чат.
test('frontend работает с настоящими Go API, PostgreSQL и фоновым обработчиком', async ({
  page,
  request,
}) => {
  const base = process.env.LIVE_API_URL
  const token = process.env.LIVE_ML_TOOLS_TOKEN
  test.skip(!base || !token, 'Нужны LIVE_API_URL и LIVE_ML_TOOLS_TOKEN отдельного тестового API')
  test.setTimeout(50_000)
  const browserHeaders: Record<string, string>[] = []
  page.on('request', (r) => {
    if (r.url().includes('/api/v1/')) browserHeaders.push(r.headers())
  })
  let fixture: unknown[] = []
  await page.route('**/api/v1/onboarding/tasks/working-accesses/chat', (route) =>
    route.fulfill({ json: { taskId: 'working-accesses', messages: fixture } }),
  )
  await page.goto('/')
  await page.getByRole('button', { name: 'Войти стажёром', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
  for (const [index, resourceId] of ['gitlab-search-api', 'grafana-search'].entries()) {
    const prepared = await request.post(`${base}/api/v1/internal/ml/tools/prepare_access_request`, {
      headers: { Authorization: `Bearer ${token}`, 'X-Demo-Employee-Id': 'intern-alina' },
      data: { resourceId },
    })
    expect(prepared.ok()).toBe(true)
    const preparation = await prepared.json()
    expect(preparation.status).toBe('draft')
    fixture = [
      {
        id: `message-${resourceId}`,
        role: 'assistant',
        text: 'Проверь параметры перед отправкой.',
        createdAt: new Date().toISOString(),
        accessDraft: preparation.draft,
      },
    ]
    await page.getByRole('button', { name: 'Мой план', exact: true }).click()
    await page.locator('.task-row').filter({ hasText: 'Рабочие доступы' }).click()
    await page.getByRole('tab', { name: 'Чат с AI-buddy' }).click()
    await page.getByRole('button', { name: 'Подтвердить отправку', exact: true }).click()
    await expect(page.locator('.draft-card')).toContainText('Заявка отправлена')
    await page.getByRole('button', { name: 'Открыть профиль', exact: true }).click()
    await page.getByRole('button', { name: 'Выйти', exact: true }).click()
    await page.getByRole('button', { name: 'Войти руководителем', exact: true }).click()
    await page.locator('.req-row').first().click()
    await page.getByRole('dialog').getByRole('button', { name: 'Одобрить', exact: true }).click()
    await expect(page.getByRole('dialog').getByText('Доступ выдан', { exact: true })).toBeVisible()
    await page.getByRole('dialog').getByRole('button', { name: 'Закрыть', exact: true }).click()
    await page.getByRole('button', { name: 'Открыть профиль', exact: true }).click()
    await page.getByRole('button', { name: 'Выйти', exact: true }).click()
    await page.getByRole('button', { name: 'Войти стажёром', exact: true }).click()
    await page.locator('.task-row').filter({ hasText: 'Рабочие доступы' }).click()
    await expect(page.locator('.resources-panel')).toContainText(`${index + 3} из 4 выдано`)
  }
  expect(browserHeaders.every((headers) => !headers.authorization)).toBe(true)
})
