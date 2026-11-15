import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RequestModal, RequestsPage } from './requests'
import {
  deferred,
  list,
  manager,
  mockClient,
  profile,
  request,
  resetClient,
} from '../test/fixtures'
import type { AccessRequest, RequestList } from '../lib/types'

vi.mock('../lib/client', async () => ({
  client: (await import('../test/fixtures')).mockClient,
  isDemo: true,
}))
beforeEach(resetClient)
describe('Список заявок', () => {
  it('фильтрует список стажёра и открывает карточку', async () => {
    const openRequest = vi.fn()
    render(<RequestsPage profile={profile} openRequest={openRequest} />)
    await userEvent.click(await screen.findByRole('button', { name: /GitLab/ }))
    expect(openRequest).toHaveBeenCalledWith(request.id)
    await userEvent.click(screen.getByRole('button', { name: 'В работе' }))
    await waitFor(() =>
      expect(mockClient.requests).toHaveBeenLastCalledWith(profile.employeeId, 'active', 0),
    )
    mockClient.requests.mockResolvedValue({ ...list, items: [], total: 0 })
    await userEvent.click(screen.getByRole('button', { name: 'Обработаны' }))
    expect(
      await screen.findByText('В этой группе пока нет заявок. Посмотри остальные.'),
    ).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Все' }))
    expect(await screen.findByText(/Попроси AI-buddy подготовить доступ/)).toBeVisible()
  })
  it('переключает страницы и сбрасывает страницу при смене фильтра', async () => {
    mockClient.requests.mockImplementation(async (_id, _group, offset = 0) => ({
      ...list,
      offset,
      total: 21,
    }))
    render(<RequestsPage profile={profile} openRequest={vi.fn()} />)
    expect(await screen.findByRole('button', { name: 'Назад' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Далее' }))
    await waitFor(() =>
      expect(mockClient.requests).toHaveBeenLastCalledWith(profile.employeeId, 'all', 20),
    )
    expect(await screen.findByText('Страница 2')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Далее' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Назад' }))
    await waitFor(() =>
      expect(mockClient.requests).toHaveBeenLastCalledWith(profile.employeeId, 'all', 0),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Далее' }))
    await userEvent.click(screen.getByRole('button', { name: 'В работе' }))
    await waitFor(() =>
      expect(mockClient.requests).toHaveBeenLastCalledWith(profile.employeeId, 'active', 0),
    )
  })
  it('руководителю показывает очередь без фильтров и имя сотрудника', async () => {
    const pending = deferred<RequestList>()
    mockClient.requests.mockReturnValueOnce(pending.promise)
    render(<RequestsPage profile={manager} openRequest={vi.fn()} />)
    expect(screen.getByText('Очередь согласования')).toBeVisible()
    expect(screen.getByRole('status')).toBeVisible()
    pending.resolve(list)
    expect(await screen.findByText('1 на согласовании')).toBeVisible()
    expect(screen.getByText(/Алина Смирнова/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Обработаны' })).not.toBeInTheDocument()
  })
  it('повторяет загрузку очереди после ошибки и показывает пустой список', async () => {
    mockClient.requests
      .mockRejectedValueOnce(new Error('Ошибка загрузки'))
      .mockResolvedValue({ ...list, items: [], total: 0, counts: { ...list.counts, pending: 0 } })
    render(<RequestsPage profile={manager} openRequest={vi.fn()} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Ошибка загрузки')
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(await screen.findByText('Все заявки рассмотрены')).toBeVisible()
    expect(screen.getByText('0 на согласовании')).toBeVisible()
  })
})
describe('Решение и карточка заявки', () => {
  const props = () => ({
    profile: manager,
    requestId: request.id,
    onClose: vi.fn(),
    onChange: vi.fn(),
  })
  it('одобряет одну заявку и обновляет данные после ответа', async () => {
    const p = props()
    const pending = deferred<AccessRequest>()
    mockClient.decide.mockReturnValueOnce(pending.promise)
    render(<RequestModal {...p} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Одобрить' }))
    expect(mockClient.decide).toHaveBeenCalledExactlyOnceWith(
      manager.employeeId,
      request.id,
      'approve',
      '',
    )
    expect(screen.getByRole('button', { name: 'Сохраняем…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Отклонить' })).toBeDisabled()
    mockClient.request.mockResolvedValue({
      ...request,
      status: 'issuing',
      decidedAt: request.createdAt,
      decisionReason: 'Ок',
    })
    pending.resolve({ ...request, status: 'issuing' })
    await waitFor(() => expect(p.onChange).toHaveBeenCalledTimes(1))
    expect(await screen.findByText('Выдаётся')).toBeVisible()
    expect(screen.getByText('Комментарий руководителя')).toBeVisible()
    expect(screen.getByText(/Решение принято/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Одобрить' })).not.toBeInTheDocument()
  })
  it('требует причину отказа и позволяет вернуться к выбору решения', async () => {
    const p = props()
    render(<RequestModal {...p} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Отклонить' }))
    expect(screen.getByRole('button', { name: 'Отклонить заявку' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Назад' }))
    await userEvent.click(screen.getByRole('button', { name: 'Отклонить' }))
    await userEvent.type(screen.getByLabelText('Причина отказа для стажёра'), '  Уточни цель  ')
    mockClient.request.mockResolvedValue({
      ...request,
      status: 'rejected',
      decisionReason: 'Уточни цель',
    })
    await userEvent.click(screen.getByRole('button', { name: 'Отклонить заявку' }))
    expect(mockClient.decide).toHaveBeenCalledWith(
      manager.employeeId,
      request.id,
      'reject',
      'Уточни цель',
    )
    expect(await screen.findByText('Причина отказа')).toBeVisible()
    expect(screen.getByText('Уточни цель')).toBeVisible()
  })
  it.each([new Error('Решение не сохранено'), null])(
    'показывает ошибку решения и сохраняет возможность повтора (%s)',
    async (error) => {
      mockClient.decide.mockRejectedValueOnce(error)
      const p = props()
      render(<RequestModal {...p} />)
      await userEvent.click(await screen.findByRole('button', { name: 'Одобрить' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось сохранить решение.',
      )
      expect(p.onChange).not.toHaveBeenCalled()
      await userEvent.click(screen.getByRole('button', { name: 'Одобрить' }))
      await waitFor(() => expect(p.onChange).toHaveBeenCalledTimes(1))
    },
  )
  it('повторяет неудачную выдачу и показывает подтверждённый результат', async () => {
    mockClient.request.mockResolvedValue({
      ...request,
      status: 'failed',
      lastError: 'Сервис недоступен',
    })
    const p = props()
    render(<RequestModal {...p} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Сервис недоступен')
    mockClient.request.mockResolvedValue({
      ...request,
      status: 'granted',
      grantedAt: request.createdAt,
    })
    await userEvent.click(screen.getByRole('button', { name: 'Повторить выдачу' }))
    expect(mockClient.retry).toHaveBeenCalledWith(manager.employeeId, request.id)
    expect(await screen.findByText(/Доступ выдан /)).toBeVisible()
    await waitFor(() => expect(p.onChange).toHaveBeenCalled())
  })
  it('стажёр читает карточку без действий руководителя, неизвестный сотрудник отображается по ID', async () => {
    mockClient.request.mockResolvedValue({ ...request, employeeId: 'unknown-employee' })
    render(<RequestModal {...props()} profile={profile} />)
    expect(await screen.findByText('unknown-employee')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Одобрить' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Отклонить' })).not.toBeInTheDocument()
  })
  it('показывает загрузку, ошибку карточки и повторяет запрос', async () => {
    const pending = deferred<AccessRequest>()
    mockClient.request.mockReturnValueOnce(pending.promise)
    render(<RequestModal {...props()} />)
    expect(screen.getByRole('status')).toBeVisible()
    pending.reject(new Error('Заявка не найдена'))
    expect(await screen.findByRole('alert')).toHaveTextContent('Заявка не найдена')
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(await screen.findByText('Заявка № 1041')).toBeVisible()
  })
})
