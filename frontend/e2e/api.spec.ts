import { expect, test } from '@playwright/test'

test('использует Go-контракт, передаёт сотрудника и не подменяет отсутствующий чат демо-ответами', async ({
  page,
}) => {
  const calls: { path: string; body: string | null; employee?: string; authorization?: string }[] =
    []
  const profile = {
    id: 'profile-alina',
    employeeId: 'intern-alina',
    firstName: 'Алина',
    secondName: 'Смирнова',
    email: 'alina.search@example.test',
    role: 'intern',
    position: 'Backend-разработчик Go',
    team: { id: 'team-search', name: 'Поиск' },
  }
  const card = {
    resources: [
      {
        id: 'gitlab-search-api',
        service: 'gitlab',
        name: 'GitLab · search-api',
        accessLevel: 'Developer',
        purpose: 'Для учебных задач',
        status: 'requestable',
        canRequest: true,
      },
    ],
    progress: { completed: 0, total: 1, inProgress: 0, failed: 0 },
  }
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname
    calls.push({
      path,
      body: request.postData(),
      employee: request.headers()['x-demo-employee-id'],
      authorization: request.headers().authorization,
    })
    if (path === '/api/v1/auth/login') return route.fulfill({ json: { status: 'ready', profile } })
    if (path === '/api/v1/onboarding/tasks/working-accesses')
      return route.fulfill({ json: { workingAccesses: card } })
    if (path === '/api/v1/access-requests')
      return route.fulfill({
        json: { items: [], total: 0, counts: { active: 0, completed: 0 }, limit: 20, offset: 0 },
      })
    return route.fulfill({ status: 404, body: '404 page not found' })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Войти стажёром', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
  await expect(page.getByText('Общий план пока недоступен.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Руководитель', exact: true })).toHaveCount(0)
  await page.locator('.task-row').filter({ hasText: 'Рабочие доступы' }).click()
  await expect(page.locator('.resource-list')).toContainText('GitLab · search-api')
  await page.getByRole('tab', { name: 'Чат с AI-buddy' }).click()
  await expect(page.getByRole('heading', { name: 'Чат пока недоступен' })).toBeVisible()
  expect(calls.find((call) => call.path.endsWith('/auth/login'))?.body).toBe(
    '{"identifier":"intern-alina"}',
  )
  expect(
    calls
      .filter((call) => !call.path.endsWith('/auth/login'))
      .every((call) => call.employee === 'intern-alina'),
  ).toBe(true)
  expect(calls.every((call) => !call.authorization && !call.path.includes('/internal/ml'))).toBe(
    true,
  )
})
