import { act, screen } from '@testing-library/react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'

vi.mock('react-dom/client', async (original) => {
  const actual = await original<typeof import('react-dom/client')>()
  return { ...actual, createRoot: vi.fn(actual.createRoot) }
})
vi.mock('./App', () => ({ default: () => <h1>Рабочее пространство AI-buddy</h1> }))

it('монтирует приложение в корневой элемент страницы', async () => {
  const container = document.createElement('div')
  container.id = 'root'
  document.body.append(container)
  try {
    await act(async () => {
      await import('./main')
    })
    expect(createRoot).toHaveBeenCalledExactlyOnceWith(container)
    expect(screen.getByRole('heading', { name: 'Рабочее пространство AI-buddy' })).toBeVisible()
  } finally {
    const root = vi.mocked(createRoot).mock.results[0]?.value
    if (root) await act(async () => root.unmount())
    container.remove()
  }
})
