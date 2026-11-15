import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useResource } from './use-resource'

describe('Загрузка данных', () => {
  it('объединяет параллельные обновления и принимает результат медленного запроса', async () => {
    let resolve!: (value: string) => void
    const load = vi.fn(
      () =>
        new Promise<string>((done) => {
          resolve = done
        }),
    )
    const { result } = renderHook(() => useResource(load))
    await waitFor(() => expect(load).toHaveBeenCalledTimes(1))
    let first!: Promise<void>, second!: Promise<void>
    act(() => {
      first = result.current.refresh()
      second = result.current.refresh()
    })
    expect(first).toBe(second)
    expect(load).toHaveBeenCalledTimes(1)
    await act(async () => {
      resolve('ready')
      await first
    })
    expect(result.current.data).toBe('ready')
    expect(result.current.loading).toBe(false)
  })
  it('игнорирует запоздалый ответ предыдущего пользователя', async () => {
    let resolveOld!: (value: string) => void
    const old = () =>
      new Promise<string>((resolve) => {
        resolveOld = resolve
      })
    const next = async () => 'new employee'
    const { result, rerender } = renderHook(({ load }) => useResource(load), {
      initialProps: { load: old },
    })
    await waitFor(() => expect(resolveOld).toBeDefined())
    rerender({ load: next })
    await waitFor(() => expect(result.current.data).toBe('new employee'))
    await act(async () => {
      resolveOld('old employee')
    })
    expect(result.current.data).toBe('new employee')
  })
})
