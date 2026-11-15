import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Avatar, Button, Empty, ErrorNotice, Icon, Loading, Modal, Status } from './ui'

describe('Общие компоненты', () => {
  it('показывает известный и незнакомый статус, инициалы и настройки кнопки', async () => {
    const click = vi.fn()
    render(
      <>
        <Status value="failed" />
        <Status value="new-status" />
        <Avatar name="Алина Смирнова" />
        <Button variant="danger" icon="bell" onClick={click}>
          Удалить
        </Button>
        <Icon name="arrow" className="extra" />
      </>,
    )
    expect(screen.getByText('Ошибка выдачи')).toHaveClass('red')
    expect(screen.getByText('new-status')).toBeVisible()
    expect(screen.getByText('АС')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    expect(click).toHaveBeenCalledTimes(1)
  })
  it('показывает пустое состояние, свою подпись загрузки и повтор ошибки', async () => {
    const retry = vi.fn()
    render(
      <>
        <Empty title="Нет данных" text="Попробуй позже">
          <span>Подсказка</span>
        </Empty>
        <Loading label="Готовим…" />
        <ErrorNotice message="Ошибка" retry={retry} />
      </>,
    )
    expect(screen.getByRole('heading', { name: 'Нет данных' })).toBeVisible()
    expect(screen.getByText('Подсказка')).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent('Готовим…')
    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(retry).toHaveBeenCalledTimes(1)
  })
  it('открывает диалог, сохраняет прокрутку и закрывает по кнопке, фону или cancel', async () => {
    document.body.style.overflow = 'auto'
    const close = vi.fn()
    const view = render(
      <Modal title="Параметры" onClose={close}>
        <p>Содержимое</p>
      </Modal>,
    )
    const dialog = screen.getByRole('dialog', { name: 'Параметры' })
    expect(dialog).toHaveAttribute('open')
    expect(document.body.style.overflow).toBe('hidden')
    await userEvent.click(screen.getByText('Содержимое'))
    expect(close).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    fireEvent.click(dialog)
    fireEvent(dialog, new Event('cancel', { bubbles: false }))
    expect(close).toHaveBeenCalledTimes(3)
    view.unmount()
    expect(document.body.style.overflow).toBe('auto')
    document.body.style.overflow = ''
  })
})
