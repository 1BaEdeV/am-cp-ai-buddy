import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// jsdom не реализует нативные диалоги и прокрутку браузера.
HTMLDialogElement.prototype.showModal = function () {
  this.setAttribute('open', '')
}
HTMLDialogElement.prototype.close = function () {
  this.removeAttribute('open')
}
Element.prototype.scrollIntoView = vi.fn()
window.scrollTo = vi.fn()

afterEach(cleanup)
