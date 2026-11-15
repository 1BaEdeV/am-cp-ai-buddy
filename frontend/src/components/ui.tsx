import { useEffect, useRef, type ButtonHTMLAttributes, type ReactNode } from 'react'

const paths = {
  arrow: <path d="M4 12h15m-6-6 6 6-6 6" />,
  back: (
    <path
      d="M7.95 19.91 0 11.95 7.95 4l1.37 1.35L3.69 11h14.58v1.95H3.69l5.63 5.61-1.37 1.35Z"
      fill="currentColor"
      stroke="none"
    />
  ),
  chevron: <path d="m9 6 6 6-6 6" />,
  check: <path d="m5 12 4 4L19 6" />,
  people: (
    <>
      <circle cx="9" cy="8" r="3" />
      <path d="M3 20v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2m2-15a3 3 0 0 1 0 6m0 3a4 4 0 0 1 4 4v2" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 8-8m-3 3 3 3m-1-5 2 2" />
    </>
  ),
  flag: <path d="M5 21V4m0 1c4-4 8 3 14-1v10c-6 4-10-3-14 1" />,
  lock: (
    <>
      <rect x="5" y="10" width="14" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3m-4 5v2" />
    </>
  ),
  calendar: (
    <>
      <rect x="3" y="5" width="18" height="16" rx="3" />
      <path d="M7 3v4m10-4v4M3 10h18m-13 5h2m4 0h2" />
    </>
  ),
  route: (
    <>
      <circle cx="5" cy="6" r="2" />
      <circle cx="19" cy="18" r="2" />
      <path d="M7 6h10a3 3 0 0 1 0 6H7a3 3 0 0 0 0 6h10" />
    </>
  ),
  list: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M9 8h6m-6 4h6m-6 4h4" />
    </>
  ),
  sparkle: <path d="M12 2l2.6 7.4L22 12l-7.4 2.6L12 22l-2.6-7.4L2 12l7.4-2.6L12 2Z" />,
  send: <path d="M12 20V4m-6 6 6-6 6 6" />,
  close: <path d="m6 6 12 12M18 6 6 18" />,
  logout: <path d="M9 5H4v14h5m5-14 7 7-7 7M8 12h13" />,
  refresh: <path d="M20 8a8 8 0 1 0 0 8m0-13v5h-5" />,
  bell: (
    <>
      <path d="M6.5 9.75C6.5 6.574 8.962 4 12 4s5.5 2.574 5.5 5.75v3.3c0 1.058.36 2.083 1.02 2.91l.73.915c.496.622.053 1.542-.743 1.542H5.493c-.796 0-1.239-.92-.743-1.542l.73-.915a4.67 4.67 0 0 0 1.02-2.91V9.75Z" />
      <path d="M9.75 20c.52.63 1.307 1 2.25 1s1.73-.37 2.25-1" />
    </>
  ),
}
export type IconName = keyof typeof paths
export function Icon({ name, className = '' }: { name: IconName; className?: string }) {
  return (
    <svg
      className={`icon ${className}`}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  )
}
export function Logo() {
  return (
    <span className="brand">
      <img src="/favicon.svg" width="32" height="32" alt="" />
      AI-buddy<span className="brand-dot">.</span>
    </span>
  )
}
export function Button({
  children,
  variant = '',
  icon,
  className = '',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: '' | 'primary' | 'danger'
  icon?: IconName
}) {
  return (
    <button type="button" className={`btn ${variant} ${className}`} {...props}>
      {children}
      {icon && <Icon name={icon} />}
    </button>
  )
}
export function Tile({ name, tone = '' }: { name: IconName; tone?: string }) {
  return (
    <span className={`task-icon ${tone || name}`}>
      <Icon name={name} />
    </span>
  )
}
export function Avatar({ name, tone = '' }: { name: string; tone?: string }) {
  return (
    <span className={`avatar ${tone}`}>
      {name
        .split(' ')
        .filter(Boolean)
        .slice(0, 2)
        .map((part) => part[0])
        .join('')}
    </span>
  )
}
export function Badge({ children, tone = '' }: { children: ReactNode; tone?: string }) {
  return <span className={`badge ${tone}`}>{children}</span>
}
const statuses: Record<string, { label: string; tone: string }> = {
  pending: { label: 'На согласовании', tone: 'orange' },
  issuing: { label: 'Выдаётся', tone: 'blue' },
  granted: { label: 'Доступ выдан', tone: 'green' },
  rejected: { label: 'Отклонена', tone: 'red' },
  failed: { label: 'Ошибка выдачи', tone: 'red' },
  requestable: { label: 'Нужен доступ', tone: '' },
  not_started: { label: 'Не начато', tone: '' },
  in_progress: { label: 'В процессе', tone: 'blue' },
  completed: { label: 'Выполнено', tone: 'green' },
  locked: { label: 'Пока закрыто', tone: '' },
  coming_soon: { label: 'Скоро', tone: 'blue' },
}
export function Status({ value }: { value: string }) {
  const status = statuses[value] || { label: value, tone: '' }
  return <Badge tone={status.tone}>{status.label}</Badge>
}
export function Loading({ label = 'Загружаем данные…' }: { label?: string }) {
  return (
    <div className="loading" role="status">
      <span className="spinner" />
      {label}
    </div>
  )
}
export function ErrorNotice({ message, retry }: { message: string; retry?: () => void }) {
  return (
    <div className="error-notice" role="alert">
      <p>{message}</p>
      {retry && (
        <Button onClick={retry} icon="refresh">
          Повторить
        </Button>
      )}
    </div>
  )
}
export function Empty({
  title,
  text,
  icon = 'list',
  children,
}: {
  title: string
  text: string
  icon?: IconName
  children?: ReactNode
}) {
  return (
    <div className="empty">
      <Tile name={icon} />
      <h3>{title}</h3>
      <p className="muted">{text}</p>
      {children}
    </div>
  )
}
export function Modal({
  title,
  children,
  onClose,
}: {
  title: string
  children: ReactNode
  onClose: () => void
}) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = ref.current!
    dialog.showModal()
    const before = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      dialog.close()
      document.body.style.overflow = before
    }
  }, [])
  return (
    <dialog
      ref={ref}
      onCancel={onClose}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
      aria-labelledby="modal-title"
    >
      <div className="dialog-head">
        <h2 id="modal-title">{title}</h2>
        <button className="close" onClick={onClose} aria-label="Закрыть">
          <Icon name="close" />
        </button>
      </div>
      <div className="dialog-body">{children}</div>
    </dialog>
  )
}
