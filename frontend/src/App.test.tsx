import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import {
  accesses,
  deferred,
  manager,
  mockClient,
  plan,
  profile,
  request,
  resetClient,
} from './test/fixtures'
import type { WorkingAccesses } from './lib/types'

const mode = vi.hoisted(() => ({ demo: true }))
vi.mock('./lib/client', async () => ({
  client: (await import('./test/fixtures')).mockClient,
  get isDemo() {
    return mode.demo
  },
}))
beforeEach(() => {
  resetClient()
  mode.demo = true
  history.replaceState(null, '', '/')
})
async function login() {
  render(<App />)
  await userEvent.click(screen.getByRole('button', { name: 'Войти стажёром' }))
  await screen.findByRole('heading', { name: 'Привет, Алина!' })
}
async function route(hash: string) {
  await act(async () => {
    location.hash = hash
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  })
}
describe('Рабочее пространство', () => {
  it('открывает план, знакомство, доступы и заявки через навигацию', async () => {
    await login()
    expect(screen.queryByText('Обсудить первые дни')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('link', { name: 'К содержимому' }))
    expect(document.getElementById('main-content')).toHaveFocus()
    await userEvent.click(screen.getByRole('button', { name: 'Продолжить адаптацию' }))
    expect(await screen.findByRole('heading', { name: 'Знакомство с командой' })).toBeVisible()
    await userEvent.click(await screen.findByRole('button', { name: 'Отметить знакомство' }))
    await waitFor(() => expect(mockClient.accesses.mock.calls.length).toBeGreaterThan(1))
    await userEvent.click(screen.getByRole('button', { name: /К плану адаптации/ }))
    expect(await screen.findByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: /ШАГ 2/ }))
    expect(await screen.findByRole('heading', { name: 'Ресурсы команды' })).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '№ 1042' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Заявка № 1041')
    expect(location.hash).toContain('request=request-2')
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Закрыть' }),
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Мои заявки' }))
    expect(await screen.findByRole('heading', { name: 'Мои заявки', level: 1 })).toBeVisible()
    await userEvent.click(await screen.findByRole('button', { name: /GitLab/ }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Заявка № 1041')
    await route('requests')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'AI-buddy, на главную' }))
    expect(await screen.findByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Мой план' }))
    await userEvent.click(screen.getByRole('button', { name: 'Все заявки' }))
    expect(await screen.findByRole('heading', { name: 'Мои заявки', level: 1 })).toBeVisible()
  })
  it('показывает встречу, профиль и выходит с очисткой маршрута', async () => {
    await login()
    await userEvent.click(screen.getByRole('button', { name: 'Подробнее о встрече' }))
    const meeting = screen.getByRole('dialog')
    expect(meeting).toHaveTextContent('Катя Волкова')
    expect(meeting).toHaveTextContent('Обсудить первые дни')
    expect(meeting).toHaveTextContent('МСК')
    await userEvent.click(within(meeting).getByRole('button', { name: 'Понятно' }))
    await userEvent.click(screen.getByRole('button', { name: 'Открыть профиль' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('alina@example.test')
    expect(screen.getByRole('dialog')).toHaveTextContent('Катя Волкова')
    await userEvent.click(screen.getByRole('button', { name: 'Понятно' }))
    await userEvent.click(screen.getByRole('button', { name: 'Открыть профиль' }))
    await route('working-accesses')
    await userEvent.click(screen.getByRole('button', { name: 'Выйти' }))
    expect(screen.getByRole('heading', { name: 'Давай познакомимся' })).toBeVisible()
    expect(location.hash).toBe('')
  })
  it('переключает роли и показывает руководителю только очередь', async () => {
    await login()
    mockClient.login.mockResolvedValue({ status: 'ready', profile: manager })
    await userEvent.click(screen.getByRole('button', { name: 'Руководитель' }))
    expect(await screen.findByRole('heading', { name: 'Заявки команды' })).toBeVisible()
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Заявки команды' }))
    await userEvent.click(screen.getByRole('button', { name: 'Открыть профиль' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('Руководитель')
    expect(screen.getByRole('dialog')).not.toHaveTextContent('Катя Волкова')
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    mockClient.login.mockResolvedValue({ status: 'ready', profile })
    await userEvent.click(screen.getByRole('button', { name: 'Стажёр' }))
    expect(await screen.findByRole('heading', { name: 'Мои заявки', level: 1 })).toBeVisible()
  })
  it.each([new Error('Не удалось сменить роль'), null])(
    'сообщает об ошибке смены роли (%s)',
    async (error) => {
      await login()
      mockClient.login.mockRejectedValueOnce(error)
      await userEvent.click(screen.getByRole('button', { name: 'Руководитель' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось сменить сотрудника.',
      )
    },
  )
  it('не меняет пользователя, если для новой роли нужна регистрация', async () => {
    await login()
    mockClient.login.mockResolvedValue((await import('./test/fixtures')).registration)
    await userEvent.click(screen.getByRole('button', { name: 'Руководитель' }))
    expect(screen.getByRole('heading', { name: 'Привет, Алина!' })).toBeVisible()
  })
  it('сохраняет ссылку на заявку при входе и закрывает карточку без потери раздела', async () => {
    history.replaceState(null, '', '/#requests?request=request-1')
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'Войти стажёром' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Заявка № 1041')
    expect(mockClient.request).toHaveBeenCalledWith(profile.employeeId, request.id)
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    await waitFor(() => expect(location.hash).toBe('#requests'))
  })
  it('использует план по умолчанию при неизвестном маршруте и ошибке API плана', async () => {
    history.replaceState(null, '', '/#unknown')
    mode.demo = false
    mockClient.plan.mockRejectedValueOnce(new Error('Нет плана'))
    const pending = deferred<WorkingAccesses>()
    mockClient.accesses.mockReturnValueOnce(pending.promise)
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'Войти стажёром' }))
    expect(await screen.findByText('Готовим твой план…')).toBeVisible()
    pending.resolve(accesses)
    expect(await screen.findByText(/Общий план пока недоступен/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Руководитель' })).not.toBeInTheDocument()
    expect(screen.getByText(profile.team.name, { selector: '.foot span' })).toBeVisible()
    mockClient.plan.mockResolvedValue(plan)
    await userEvent.click(screen.getByRole('button', { name: 'Обновить план' }))
    expect(await screen.findByRole('button', { name: 'Подробнее о встрече' })).toBeVisible()
  })
  it.each([
    undefined,
    { ...accesses, progress: { completed: 0, total: 3, inProgress: 0, failed: 0 } },
    { ...accesses, progress: { completed: 3, total: 3, inProgress: 0, failed: 0 } },
  ])(
    'сохраняет доступ к плану при сбое данных и корректно считает запасной прогресс (%j)',
    async (card) => {
      mockClient.plan.mockRejectedValue(new Error('Нет плана'))
      if (card) mockClient.accesses.mockResolvedValue(card)
      else mockClient.accesses.mockRejectedValueOnce(new Error('Нет ресурсов'))
      await login()
      expect(await screen.findByText(/Общий план пока недоступен/)).toBeVisible()
      if (!card) {
        expect(screen.getByRole('alert')).toHaveTextContent('Нет ресурсов')
        await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
        await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
      }
      await userEvent.click(screen.getByRole('button', { name: 'Обновить план' }))
      expect(mockClient.plan.mock.calls.length).toBeGreaterThan(1)
    },
  )
  it('обновляет план и показывает пустую встречу при её отсутствии', async () => {
    mockClient.plan.mockResolvedValue({ ...plan, upcomingMeeting: undefined })
    await login()
    expect(screen.getByText(/ближайшая встреча/)).toBeVisible()
    const emptyMeeting = { ...plan, upcomingMeeting: undefined }
    // Уже открытая карточка переживает обновление плана без встречи.
    mockClient.plan.mockResolvedValue(plan)
    fireEvent.focus(window)
    await userEvent.click(await screen.findByRole('button', { name: 'Подробнее о встрече' }))
    mockClient.plan.mockResolvedValue(emptyMeeting)
    fireEvent.focus(window)
    expect(await screen.findByText('Ближайшая встреча пока не назначена.')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
