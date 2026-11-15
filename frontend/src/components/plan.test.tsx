import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Plan, TaskPage } from './plan'
import { accesses, mockClient, plan, profile, resetClient } from '../test/fixtures'
import type { TaskId } from '../lib/types'

vi.mock('../lib/client', async () => ({
  client: (await import('../test/fixtures')).mockClient,
  isDemo: true,
}))
beforeEach(resetClient)
function taskProps(taskId: TaskId) {
  return {
    taskId,
    profile,
    plan,
    accesses,
    requests: [],
    navigate: vi.fn(),
    onChange: vi.fn(),
    openRequest: vi.fn(),
  }
}
describe('План и задания', () => {
  it('показывает единый список, прогресс доступов и переходы к шагам', async () => {
    const navigate = vi.fn()
    const view = render(
      <Plan profile={profile} plan={plan} accesses={accesses} navigate={navigate} />,
    )
    expect(screen.getByText(/1 из 3 доступов выдано/)).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: /Продолжить адаптацию/ }))
    expect(navigate).toHaveBeenLastCalledWith('team-intro')
    await userEvent.click(screen.getByRole('button', { name: /ШАГ 2/ }))
    expect(navigate).toHaveBeenLastCalledWith('working-accesses')
    view.rerender(<Plan profile={profile} plan={plan} navigate={navigate} />)
    expect(screen.getByText('Ресурсы')).toBeVisible()
  })
  it('показывает карточки ресурсов, открывает существующую заявку и обсуждение нового доступа', async () => {
    const props = taskProps('working-accesses')
    render(<TaskPage {...props} />)
    expect(screen.getByText('Вики')).toBeVisible()
    expect(screen.getByText('1 из 3 выдано')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '№ 1042' }))
    expect(props.openRequest).toHaveBeenCalledWith('request-2')
    await userEvent.click(screen.getByRole('button', { name: /Обсудить с AI-buddy/ }))
    expect(await screen.findByRole('log')).toBeVisible()
    await userEvent.click(screen.getByRole('tab', { name: 'Задание' }))
    await userEvent.click(screen.getByRole('button', { name: 'Открыть чат' }))
    expect(screen.getByRole('tab', { name: /Чат с AI-buddy/ })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await userEvent.click(screen.getByRole('button', { name: /К плану адаптации/ }))
    expect(props.navigate).toHaveBeenCalledWith('plan')
  })
  it('ожидает ресурсы и отмечает полностью выданные доступы', () => {
    const props = taskProps('working-accesses')
    const view = render(<TaskPage {...props} accesses={undefined} />)
    expect(screen.getByRole('status')).toHaveTextContent('Загружаем')
    view.rerender(
      <TaskPage
        {...props}
        accesses={{ resources: [], progress: { completed: 3, total: 3, inProgress: 0, failed: 0 } }}
      />,
    )
    expect(screen.getByText('3 из 3 выдано')).toHaveClass('green')
  })
  it('загружает людей команды и отмечает знакомство', async () => {
    const props = taskProps('team-intro')
    render(<TaskPage {...props} />)
    expect(await screen.findByText('Катя Волкова')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Отметить знакомство' }))
    expect(mockClient.completeIntro).toHaveBeenCalledWith(profile.employeeId)
    await waitFor(() => expect(props.onChange).toHaveBeenCalledTimes(1))
    await userEvent.click(screen.getByRole('button', { name: 'Спросить AI-buddy' }))
    expect(await screen.findByRole('log')).toBeVisible()
  })
  it.each([new Error('Не удалось сохранить'), null])(
    'даёт повторить отметку знакомства после ошибки (%s)',
    async (error) => {
      mockClient.completeIntro.mockRejectedValueOnce(error)
      const props = taskProps('team-intro')
      render(<TaskPage {...props} />)
      await userEvent.click(await screen.findByRole('button', { name: 'Отметить знакомство' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось отметить знакомство.',
      )
      expect(props.onChange).not.toHaveBeenCalled()
      await userEvent.click(screen.getByRole('button', { name: 'Отметить знакомство' }))
      await waitFor(() => expect(props.onChange).toHaveBeenCalledTimes(1))
    },
  )
  it('повторяет загрузку знакомства и скрывает повторное завершение уже выполненного шага', async () => {
    mockClient.intro.mockRejectedValueOnce(new Error('Нет связи'))
    const props = taskProps('team-intro')
    render(
      <TaskPage
        {...props}
        plan={{
          ...plan,
          tasks: plan.tasks.map((t) => (t.id === 'team-intro' ? { ...t, status: 'completed' } : t)),
        }}
      />,
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('Нет связи')
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(await screen.findByText('Знакомство отмечено')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Отметить знакомство' })).not.toBeInTheDocument()
  })
  it('закрывает первую задачу до подготовки и показывает ожидание задания после подготовки', async () => {
    const props = taskProps('first-task')
    const view = render(<TaskPage {...props} />)
    expect(screen.getByText('Сначала — немного подготовки')).toBeVisible()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Перейти к текущему шагу' }))
    expect(props.navigate).toHaveBeenCalledWith('team-intro')
    view.rerender(
      <TaskPage
        {...props}
        plan={{
          ...plan,
          tasks: plan.tasks.map((t) =>
            t.id === 'first-task' ? { ...t, status: 'coming_soon' } : t,
          ),
        }}
      />,
    )
    expect(screen.getByText('Здесь появится первая задача')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Перейти к текущему шагу' }),
    ).not.toBeInTheDocument()
  })
})
