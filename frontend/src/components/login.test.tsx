import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Login } from './login'
import { deferred, mockClient, profile, registration, resetClient } from '../test/fixtures'

const mode = vi.hoisted(() => ({ demo: true }))
vi.mock('../lib/client', async () => ({
  client: (await import('../test/fixtures')).mockClient,
  get isDemo() {
    return mode.demo
  },
}))
beforeEach(() => {
  resetClient()
  mode.demo = true
})

describe('Вход и регистрация', () => {
  it('при смене команды предлагает её должности и наставников и сохраняет выбранного', async () => {
    const search = {
      id: 'search',
      name: 'Поиск',
      positionOptions: [{ id: 'go', name: 'Go-разработчик' }],
      mentors: [
        { id: 'katya', firstName: 'Катя', secondName: 'Волкова' },
        { id: 'sasha', firstName: 'Саша', secondName: 'Лебедев' },
      ],
    }
    mockClient.login.mockResolvedValue({
      ...registration,
      teamOptions: [registration.team, search],
    })
    render(<Login onLogin={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Иван Петров' }))
    await screen.findByLabelText('Команда')
    expect(screen.getByLabelText('Наставник')).toHaveValue('oleg')
    await userEvent.selectOptions(screen.getByLabelText('Команда'), 'search')
    expect(screen.getByLabelText('Должность')).toHaveValue('go')
    expect(screen.getByLabelText('Наставник')).toHaveValue('katya')
    expect(screen.queryByRole('option', { name: 'Олег Морозов' })).not.toBeInTheDocument()
    await userEvent.selectOptions(screen.getByLabelText('Наставник'), 'sasha')
    fireEvent.change(screen.getByLabelText('Начало стажировки'), {
      target: { value: '2026-11-16' },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Открыть мой план' }))
    expect(mockClient.register).toHaveBeenCalledWith('intern-ivan', {
      teamId: 'search',
      positionId: 'go',
      mentorId: 'sasha',
      internshipStartDate: '2026-11-16',
    })
  })
  it('в API-режиме позволяет выбрать другие тестовые профили', async () => {
    mode.demo = false
    mockClient.login.mockResolvedValue(registration)
    render(<Login onLogin={vi.fn()} />)
    await userEvent.selectOptions(screen.getByLabelText('Другие тестовые стажёры'), 'intern-maria')
    expect(mockClient.login).toHaveBeenCalledWith('intern-maria')
    expect(await screen.findByText('Привет, Иван!')).toBeVisible()
  })
  it('отправляет введённый ID без пробелов и блокирует повторный вход до ответа', async () => {
    const pending = deferred<Awaited<ReturnType<typeof mockClient.login>>>()
    mockClient.login.mockReturnValue(pending.promise)
    const onLogin = vi.fn()
    render(<Login onLogin={onLogin} />)
    expect(screen.getByRole('button', { name: 'Продолжить' })).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Рабочая почта или ID'), {
      target: { value: ' intern-alina ' },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Продолжить' }))
    expect(mockClient.login).toHaveBeenCalledExactlyOnceWith('intern-alina')
    expect(screen.getByRole('button', { name: 'Входим…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Войти руководителем' })).toBeDisabled()
    pending.resolve({ status: 'ready', profile })
    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(profile))
  })
  it.each([new Error('Сотрудник не найден'), null])(
    'показывает ошибку входа и позволяет повторить его (%s)',
    async (error) => {
      mockClient.login.mockRejectedValueOnce(error)
      const onLogin = vi.fn()
      render(<Login onLogin={onLogin} />)
      await userEvent.click(screen.getByRole('button', { name: 'Войти стажёром' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось войти.',
      )
      await userEvent.click(screen.getByRole('button', { name: 'Войти руководителем' }))
      await waitFor(() => expect(onLogin).toHaveBeenCalledWith(profile))
      expect(mockClient.login).toHaveBeenLastCalledWith('manager-dmitry')
    },
  )
  it('возвращается из регистрации к выбору сотрудника', async () => {
    mockClient.login.mockResolvedValue(registration)
    render(<Login onLogin={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Иван Петров' }))
    expect(await screen.findByText('Привет, Иван!')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Другой сотрудник' }))
    expect(screen.getByLabelText('Рабочая почта или ID')).toBeVisible()
  })
  it.each([new Error('Профиль не сохранён'), null])(
    'сохраняет профиль стажёра после исправления ошибки (%s)',
    async (error) => {
      mockClient.login.mockResolvedValue(registration)
      mockClient.register.mockRejectedValueOnce(error)
      const onLogin = vi.fn()
      render(<Login onLogin={onLogin} />)
      await userEvent.click(screen.getByRole('button', { name: 'Иван Петров' }))
      await screen.findByLabelText('Должность')
      await userEvent.selectOptions(screen.getByLabelText('Должность'), 'backend')
      await userEvent.selectOptions(screen.getByLabelText('Наставник'), 'oleg')
      fireEvent.change(screen.getByLabelText('Начало стажировки'), {
        target: { value: '2026-11-16' },
      })
      await userEvent.click(screen.getByRole('button', { name: 'Открыть мой план' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(
        error instanceof Error ? error.message : 'Не удалось сохранить профиль.',
      )
      expect(onLogin).not.toHaveBeenCalled()
      await userEvent.click(screen.getByRole('button', { name: 'Открыть мой план' }))
      expect(mockClient.register).toHaveBeenLastCalledWith('intern-ivan', {
        positionId: 'backend',
        teamId: 'web',
        mentorId: 'oleg',
        internshipStartDate: '2026-11-16',
      })
      await waitFor(() => expect(onLogin).toHaveBeenCalledWith(profile))
    },
  )
  it('для руководителя не отправляет поля стажировки', async () => {
    mockClient.login.mockResolvedValue({
      ...registration,
      role: 'manager',
      team: { ...registration.team, mentors: [] },
    })
    const onLogin = vi.fn()
    render(<Login onLogin={onLogin} />)
    await userEvent.click(screen.getByRole('button', { name: 'Войти руководителем' }))
    await screen.findByLabelText('Должность')
    expect(screen.queryByLabelText('Наставник')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Открыть мой план' }))
    expect(mockClient.register).toHaveBeenCalledWith('intern-ivan', { positionId: 'frontend' })
    await waitFor(() => expect(onLogin).toHaveBeenCalled())
  })
})
