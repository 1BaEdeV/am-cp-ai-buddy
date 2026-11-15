import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Chat } from './chat'
import {
  accesses,
  deferred,
  draft,
  mockClient,
  profile,
  request,
  resetClient,
} from '../test/fixtures'
import type { AccessRequest, TaskChat } from '../lib/types'

vi.mock('../lib/client', async () => ({
  client: (await import('../test/fixtures')).mockClient,
  isDemo: true,
}))
beforeEach(resetClient)
afterEach(() => vi.useRealTimers())
const props = () => ({
  profile,
  taskId: 'working-accesses' as const,
  resources: accesses.resources,
  requests: [],
  onChange: vi.fn(),
  openRequest: vi.fn(),
})
function draftChat(expiresAt = draft.expiresAt): TaskChat {
  return {
    taskId: 'working-accesses',
    messages: [
      {
        id: 'message-1',
        role: 'assistant',
        text: 'Проверь параметры',
        createdAt: draft.createdAt,
        sourceLabel: 'Каталог',
        draft: { ...draft, expiresAt },
      },
    ],
  }
}
describe('Переписка с агентом', () => {
  it('передаёт один выбранный ресурс агенту, не создавая заявку без подтверждения', async () => {
    const p = props()
    render(<Chat {...p} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Запросить GitLab' }))
    expect(mockClient.sendMessage).toHaveBeenCalledWith(
      profile.employeeId,
      'working-accesses',
      'Запроси доступ к GitLab · search-api',
      'gitlab',
    )
    await waitFor(() => expect(p.onChange).toHaveBeenCalledTimes(1))
    expect(mockClient.confirm).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Запросить Вики' })).not.toBeInTheDocument()
  })
  it('отправляет текст без пробелов и блокирует форму до ответа', async () => {
    const pending = deferred<void>()
    mockClient.sendMessage.mockReturnValueOnce(pending.promise)
    mockClient.chat
      .mockResolvedValueOnce({ taskId: 'working-accesses', messages: [] })
      .mockResolvedValue({
        taskId: 'working-accesses',
        messages: [
          { id: 'user', role: 'user', text: 'Мой вопрос', createdAt: draft.createdAt },
          { id: 'answer', role: 'assistant', text: 'Ответ', createdAt: draft.createdAt, request },
        ],
      })
    const p = props()
    render(<Chat {...p} />)
    const input = await screen.findByRole('textbox', { name: 'Вопрос AI-buddy' })
    expect(screen.getByRole('button', { name: 'Отправить сообщение' })).toBeDisabled()
    await userEvent.type(input, '  Мой вопрос  ')
    await userEvent.click(screen.getByRole('button', { name: 'Отправить сообщение' }))
    expect(mockClient.sendMessage).toHaveBeenCalledWith(
      profile.employeeId,
      'working-accesses',
      'Мой вопрос',
      undefined,
    )
    expect(input).toBeDisabled()
    expect(screen.getByText('AI-buddy готовит ответ')).toBeInTheDocument()
    pending.resolve()
    expect(await screen.findByText('Ответ')).toBeVisible()
    expect(input).toHaveValue('')
    expect(screen.getByText('Ты')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Заявка № 1041' }))
    expect(p.openRequest).toHaveBeenCalledWith(request.id)
  })
  it.each([new Error('Не отправлено'), null])(
    'сохраняет введённый текст после ошибки отправки (%s)',
    async (error) => {
      mockClient.sendMessage.mockRejectedValueOnce(error)
      render(<Chat {...props()} />)
      const input = await screen.findByRole('textbox')
      await userEvent.type(input, 'Вопрос')
      await userEvent.click(screen.getByRole('button', { name: 'Отправить сообщение' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось отправить сообщение.',
      )
      expect(input).toHaveValue('Вопрос')
      await userEvent.click(screen.getByRole('button', { name: 'Отправить сообщение' }))
      await waitFor(() => expect(input).toHaveValue(''))
    },
  )
  it('не отправляет пустой вопрос даже при программной отправке формы', async () => {
    render(<Chat {...props()} />)
    const input = await screen.findByRole('textbox')
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.submit(input.closest('form')!)
    expect(mockClient.sendMessage).not.toHaveBeenCalled()
  })
  it('показывает вопросы о команде и отправляет подсказку', async () => {
    render(<Chat {...props()} taskId="team-intro" />)
    expect(await screen.findByText(/Привет, Алина!/)).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Кто мой наставник?' }))
    expect(mockClient.sendMessage).toHaveBeenCalledWith(
      profile.employeeId,
      'team-intro',
      'Кто мой наставник?',
      undefined,
    )
  })
  it.each(['queued', 'running'] as const)(
    'блокирует отправку при состоянии агента %s',
    async (status) => {
      mockClient.chat.mockResolvedValue({
        taskId: 'working-accesses',
        messages: [],
        latestRun: { id: 'run', userMessageId: 'message', status },
      })
      render(<Chat {...props()} />)
      expect(await screen.findByRole('textbox')).toBeDisabled()
      expect(screen.getByRole('button', { name: 'Запросить GitLab' })).toBeDisabled()
    },
  )
  it.each(['Ошибка агента', undefined])('объясняет сбой фонового ответа (%s)', async (error) => {
    mockClient.chat.mockResolvedValue({
      taskId: 'working-accesses',
      messages: [],
      latestRun: { id: 'run', userMessageId: 'message', status: 'failed', error },
    })
    render(<Chat {...props()} />)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      error || 'Не удалось подготовить ответ.',
    )
    expect(screen.getByRole('textbox')).toBeEnabled()
  })
  it('повторяет загрузку недоступного чата', async () => {
    mockClient.chat.mockRejectedValueOnce(new Error('Нет связи'))
    render(<Chat {...props()} />)
    expect(await screen.findByText('Чат пока недоступен')).toBeVisible()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(await screen.findByRole('textbox')).toBeEnabled()
  })
})
describe('Подтверждение черновика', () => {
  it('показывает снимок параметров и создаёт заявку только при явном согласии', async () => {
    const pending = deferred<AccessRequest>()
    mockClient.chat.mockResolvedValue(draftChat())
    mockClient.confirm.mockReturnValueOnce(pending.promise)
    const p = props()
    render(<Chat {...p} />)
    expect(await screen.findByText('Developer')).toBeVisible()
    expect(screen.getByText('Дмитрий Орлов')).toBeVisible()
    expect(screen.getByText('Работа с кодом')).toBeVisible()
    expect(mockClient.confirm).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Подтвердить отправку' }))
    expect(screen.getByRole('button', { name: 'Отправляем…' })).toBeDisabled()
    pending.resolve(request)
    expect(await screen.findByText('Заявка отправлена')).toBeVisible()
    expect(mockClient.confirm).toHaveBeenCalledExactlyOnceWith(profile.employeeId, draft.id)
    await userEvent.click(screen.getByRole('button', { name: 'Открыть заявку № 1041' }))
    expect(p.openRequest).toHaveBeenCalledWith(request.id)
    expect(p.onChange).toHaveBeenCalledTimes(1)
  })
  it('уже отправленный черновик показывает существующую заявку', async () => {
    mockClient.chat.mockResolvedValue(draftChat())
    render(<Chat {...props()} requests={[request]} />)
    expect(await screen.findByRole('button', { name: 'Открыть заявку № 1041' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Подтвердить отправку' })).not.toBeInTheDocument()
  })
  it.each([new Error('Черновик изменился'), null])(
    'показывает ошибку подтверждения без ложного успеха (%s)',
    async (error) => {
      mockClient.chat.mockResolvedValue(draftChat())
      mockClient.confirm.mockRejectedValueOnce(error)
      const p = props()
      render(<Chat {...p} />)
      await userEvent.click(await screen.findByRole('button', { name: 'Подтвердить отправку' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось отправить заявку.',
      )
      expect(screen.queryByText('Заявка отправлена')).not.toBeInTheDocument()
      expect(p.onChange).not.toHaveBeenCalled()
      await userEvent.click(screen.getByRole('button', { name: 'Подтвердить отправку' }))
      expect(await screen.findByText('Заявка отправлена')).toBeVisible()
    },
  )
  it('отключает подтверждение черновика с истёкшим сроком', async () => {
    mockClient.chat.mockResolvedValue(draftChat('2000-01-01T00:00:00Z'))
    render(<Chat {...props()} />)
    expect(await screen.findByRole('button', { name: 'Подтвердить отправку' })).toBeDisabled()
    expect(screen.getByText('Срок черновика истёк. Попроси подготовить новый.')).toBeVisible()
  })
  it('отключает подтверждение в момент истечения срока открытого черновика', async () => {
    vi.useFakeTimers()
    mockClient.chat.mockResolvedValue(draftChat(new Date(Date.now() + 1000).toISOString()))
    render(<Chat {...props()} />)
    await act(async () => {})
    expect(screen.getByRole('button', { name: 'Подтвердить отправку' })).toBeEnabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1001)
    })
    expect(screen.getByRole('button', { name: 'Подтвердить отправку' })).toBeDisabled()
    expect(mockClient.confirm).not.toHaveBeenCalled()
  })
})
