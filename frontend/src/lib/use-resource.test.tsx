import { act, fireEvent, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useResource } from './use-resource'
import { deferred } from '../test/fixtures'

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Загрузка данных', () => {
  it.each([new Error('Сервер недоступен'), null])(
    'сообщает об ошибке и очищает её после успешного обновления (%s)',
    async (error) => {
      const load = vi.fn().mockRejectedValueOnce(error).mockResolvedValue('ready')
      const { result } = renderHook(() => useResource(load))
      await waitFor(() => expect(result.current.loading).toBe(false))
      expect(result.current.error).toBe(
        error instanceof Error ? error.message : 'Не удалось загрузить данные.',
      )
      await act(async () => {
        await result.current.refresh()
      })
      expect(result.current.data).toBe('ready')
      expect(result.current.error).toBe('')
    },
  )
  it('опрашивает только видимую страницу и обновляет при возвращении фокуса', async () => {
    vi.useFakeTimers()
    const load = vi.fn().mockResolvedValue('ready')
    const visibility = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const { unmount } = renderHook(() => useResource(load, 1000))
    await act(async () => {})
    expect(load).toHaveBeenCalledTimes(1)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(load).toHaveBeenCalledTimes(2)
    visibility.mockReturnValue(true)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(load).toHaveBeenCalledTimes(2)
    await act(async () => {
      fireEvent.focus(window)
    })
    expect(load).toHaveBeenCalledTimes(3)
    unmount()
    await act(async () => {
      fireEvent.focus(window)
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(load).toHaveBeenCalledTimes(3)
  })
  it('игнорирует ошибку прошлого запроса и ответ после размонтирования', async () => {
    const old = deferred<string>(),
      next = deferred<string>()
    const loadOld = () => old.promise,
      loadNext = () => next.promise
    const { result, rerender, unmount } = renderHook(({ load }) => useResource(load), {
      initialProps: { load: loadOld },
    })
    await act(async () => {})
    rerender({ load: loadNext })
    await act(async () => {
      old.reject(new Error('Старый пользователь'))
    })
    expect(result.current.error).toBe('')
    unmount()
    await act(async () => {
      next.resolve('Поздний ответ')
    })
    expect(result.current.data).toBeUndefined()
  })
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
