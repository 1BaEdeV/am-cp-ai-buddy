import { expect, test, type Page } from '@playwright/test'

async function login(page: Page) {
  await page.goto('/')
  await page.getByRole('button', { name: 'Войти стажёром', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
}
async function openTask(page: Page, title: string) {
  await page.getByRole('button', { name: 'Мой план', exact: true }).click()
  await page.locator('.task-row').filter({ hasText: title }).click()
}
async function prepareAndConfirm(page: Page, name: RegExp) {
  await openTask(page, 'Рабочие доступы')
  await page.getByRole('tab', { name: 'Чат с AI-buddy' }).click()
  await page.getByRole('button', { name }).click()
  const draft = page.locator('.draft-card').last()
  await expect(draft).toContainText('Черновик одного доступа')
  await draft.getByRole('button', { name: 'Подтвердить отправку' }).click()
  await expect(draft).toContainText('Заявка отправлена')
}
async function approve(page: Page) {
  await page.getByRole('button', { name: 'Руководитель', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Заявки команды' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Обработаны', exact: true })).toHaveCount(0)
  await page.locator('.req-row').first().click()
  const modal = page.getByRole('dialog')
  await modal.getByRole('button', { name: 'Одобрить', exact: true }).click()
  await expect(modal.getByText('Доступ выдан', { exact: true })).toBeVisible()
  await modal.getByRole('button', { name: 'Закрыть', exact: true }).click()
  await page.getByRole('button', { name: 'Стажёр', exact: true }).click()
}
test('ссылка на заявку сохраняется после обновления и открывается согласующему', async ({
  page,
}) => {
  await login(page)
  await prepareAndConfirm(page, /^Запросить GitLab$/)
  await page
    .locator('.draft-card')
    .getByRole('button', { name: /^Открыть заявку №/ })
    .click()
  await expect(page.getByRole('dialog')).toContainText('На согласовании')
  expect(new URL(page.url()).hash).toContain('request=')
  await page.reload()
  await page.getByRole('button', { name: 'Войти руководителем', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('GitLab · search-api')
  await expect(
    page.getByRole('dialog').getByRole('button', { name: 'Одобрить', exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  expect(new URL(page.url()).hash).not.toContain('request=')
})
test('план одним списком, черновик в чате, два одобрения и прогресс 4/4', async ({
  page,
}, info) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await login(page)
  await expect(page.locator('.task-row')).toHaveCount(3)
  await expect(page.getByRole('button', { name: 'Общая', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'В команде', exact: true })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('plan.png'), fullPage: true })
  await openTask(page, 'Знакомство с командой')
  await page.getByRole('button', { name: 'Отметить знакомство', exact: true }).click()
  await expect(page.getByText('Знакомство отмечено', { exact: true })).toBeVisible()
  await prepareAndConfirm(page, /^Запросить GitLab$/)
  await page.screenshot({ path: info.outputPath('chat.png'), fullPage: true })
  await approve(page)
  await expect(page.locator('.resources-panel')).toContainText('3 из 4 выдано')
  await prepareAndConfirm(page, /^Запросить Grafana/)
  await approve(page)
  await expect(page.locator('.resources-panel')).toContainText('4 из 4 выдано')
  await page.getByRole('button', { name: 'Мой план', exact: true }).click()
  await expect(page.locator('.progress-summary')).toContainText('2 из 3 этапов')
  await openTask(page, 'Первая задача')
  await expect(page.getByRole('heading', { name: 'Здесь появится первая задача' })).toBeVisible()
  expect(errors).toEqual([])
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
})
test('руководитель отклоняет с причиной, стажёр видит её в обработанных', async ({ page }) => {
  await login(page)
  await prepareAndConfirm(page, /^Запросить GitLab$/)
  await page.getByRole('button', { name: 'Руководитель', exact: true }).click()
  await page.locator('.req-row').first().click()
  const modal = page.getByRole('dialog')
  await modal.getByRole('button', { name: 'Отклонить', exact: true }).click()
  await expect(modal.getByRole('button', { name: 'Отклонить заявку', exact: true })).toBeDisabled()
  await modal.getByLabel('Причина отказа для стажёра').fill('Уточни назначение доступа')
  await modal.getByRole('button', { name: 'Отклонить заявку', exact: true }).click()
  await expect(modal.getByText('Отклонена', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Стажёр', exact: true }).click()
  await page.getByRole('button', { name: 'Мои заявки', exact: true }).click()
  await page.getByRole('button', { name: 'Обработаны', exact: true }).click()
  await page.locator('.req-row').first().click()
  await expect(page.getByRole('dialog')).toContainText('Уточни назначение доступа')
})
test('ошибка входа, регистрация, профиль и повторный вход после обновления', async ({
  page,
}, info) => {
  await page.goto('/')
  await page.screenshot({ path: info.outputPath('login.png'), fullPage: true })
  await page.getByLabel('Рабочая почта или ID').fill('unknown')
  await page.getByRole('button', { name: 'Продолжить', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Не нашли сотрудника')
  await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click()
  await page.getByLabel('Начало стажировки').fill('2026-11-15')
  await page.getByRole('button', { name: 'Открыть мой план', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Привет, Иван!' })).toBeVisible()
  await page.getByRole('button', { name: 'Открыть профиль', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('Иван Петров')
  await page.keyboard.press('Escape')
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Давай познакомимся' })).toBeVisible()
  await page.getByRole('button', { name: 'Зарегистрироваться', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Привет, Иван!' })).toBeVisible()
})
