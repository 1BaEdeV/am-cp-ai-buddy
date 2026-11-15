import { useCallback, useState } from 'react'
import { client, isDemo } from '../lib/client'
import type { AccessRequest, Profile, RequestGroup } from '../lib/types'
import { useResource } from '../lib/use-resource'
import { Button, Empty, ErrorNotice, Icon, Loading, Modal, Status, Tile } from './ui'

export function RequestsPage({
  profile,
  openRequest,
}: {
  profile: Profile
  openRequest: (id: string) => void
}) {
  const [group, setGroup] = useState<RequestGroup>('all')
  const manager = profile.role === 'manager'
  return (
    <>
      <div className="page-head">
        <span className="eyebrow">{manager ? 'Согласование доступов' : 'Всё под рукой'}</span>
        <h1>{manager ? 'Заявки команды' : 'Мои заявки'}</h1>
        <p className="muted">
          {manager
            ? `Рабочие доступы стажёров команды «${profile.team.name}».`
            : 'Статусы рабочих доступов и история согласования.'}
        </p>
      </div>
      {!manager && (
        <div className="segments request-filters" aria-label="Фильтр заявок">
          {(
            [
              ['all', 'Все'],
              ['active', 'В работе'],
              ['completed', 'Обработаны'],
            ] as const
          ).map(([value, label]) => (
            <button key={value} aria-pressed={group === value} onClick={() => setGroup(value)}>
              {label}
            </button>
          ))}
        </div>
      )}
      <RequestRows key={group} profile={profile} group={group} openRequest={openRequest} />
    </>
  )
}
function RequestRows({
  profile,
  group,
  openRequest,
}: {
  profile: Profile
  group: RequestGroup
  openRequest: (id: string) => void
}) {
  const [offset, setOffset] = useState(0)
  const load = useCallback(
    () => client.requests(profile.employeeId, group, offset),
    [profile.employeeId, group, offset],
  )
  const { data, error, loading, refresh } = useResource(load, 2500)
  return (
    <>
      {profile.role === 'manager' && (
        <section className="panel manager-banner">
          <div>
            <h2>{data ? `${data.counts.pending} на согласовании` : 'Очередь согласования'}</h2>
            <p className="muted">Проверь ресурс, уровень доступа и основание запроса.</p>
          </div>
          <Tile name="list" tone="people" />
        </section>
      )}
      {error && <ErrorNotice message={error} retry={() => void refresh()} />}
      <section className="panel requests">
        {loading && !data ? (
          <Loading />
        ) : (
          data && (
            <>
              <div className="requests-head">
                <h3>{profile.role === 'manager' ? 'Ждут твоего решения' : 'Рабочие доступы'}</h3>
                <span className="small muted">{data.total} в списке</span>
              </div>
              {data.items.length ? (
                data.items.map((request) => (
                  <button
                    key={request.id}
                    className="req-row"
                    onClick={() => openRequest(request.id)}
                  >
                    <Tile name="key" />
                    <span className="task-text">
                      <strong>{request.parameters.resourceName}</strong>
                      <small>
                        № {request.number} ·{' '}
                        {profile.role === 'manager'
                          ? employeeName(request.employeeId)
                          : request.parameters.accessLevel}
                      </small>
                    </span>
                    <Status value={request.status} />
                    <Icon name="chevron" className="chevron" />
                  </button>
                ))
              ) : (
                <Empty
                  title={profile.role === 'manager' ? 'Все заявки рассмотрены' : 'Пока ничего нет'}
                  text={
                    profile.role === 'manager'
                      ? 'Новые запросы от стажёров появятся здесь.'
                      : group === 'all'
                        ? 'Попроси AI-buddy подготовить доступ — после подтверждения заявка появится здесь.'
                        : 'В этой группе пока нет заявок. Посмотри остальные.'
                  }
                />
              )}
              {(offset > 0 || offset + data.limit < data.total) && (
                <div className="pagination">
                  <Button
                    onClick={() => setOffset(Math.max(0, offset - data.limit))}
                    disabled={offset === 0}
                  >
                    Назад
                  </Button>
                  <span className="small muted">
                    Страница {Math.floor(offset / data.limit) + 1}
                  </span>
                  <Button
                    onClick={() => setOffset(offset + data.limit)}
                    disabled={offset + data.limit >= data.total}
                  >
                    Далее
                  </Button>
                </div>
              )}
            </>
          )
        )}
      </section>
    </>
  )
}
function employeeName(id: string) {
  return isDemo ? { 'intern-alina': 'Алина Смирнова', 'intern-ivan': 'Иван Петров' }[id] || id : id
}
const formatDate = (value: string) =>
  new Date(value).toLocaleString('ru', {
    day: 'numeric',
    month: 'long',
    hour: '2-digit',
    minute: '2-digit',
  })
export function RequestModal({
  profile,
  requestId,
  onClose,
  onChange,
}: {
  profile: Profile
  requestId: string
  onClose: () => void
  onChange: () => void
}) {
  const load = useCallback(
    () => client.request(profile.employeeId, requestId),
    [profile.employeeId, requestId],
  )
  const { data, error, loading, refresh } = useResource(load, 1500)
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState('')
  async function decide(decision: 'approve' | 'reject' | 'retry') {
    if (busy) return
    setBusy(true)
    setActionError('')
    try {
      if (decision === 'retry') await client.retry(profile.employeeId, requestId)
      else await client.decide(profile.employeeId, requestId, decision, reason.trim())
      setRejecting(false)
      await refresh()
      onChange()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось сохранить решение.')
      void refresh()
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={data ? `Заявка № ${data.number}` : 'Заявка на доступ'} onClose={onClose}>
      {loading && !data ? (
        <Loading />
      ) : (
        data && (
          <>
            <Status value={data.status} />
            <RequestParameters request={data} />
            {data.decisionReason && (
              <div className="reason">
                <strong>
                  {data.status === 'rejected' ? 'Причина отказа' : 'Комментарий руководителя'}
                </strong>
                <p>{data.decisionReason}</p>
              </div>
            )}
            {data.lastError && <ErrorNotice message={data.lastError} />}
            <div className="request-timeline">
              <p>
                <span className="dot blue" />
                Создана {formatDate(data.createdAt)}
              </p>
              {data.decidedAt && (
                <p>
                  <span className="dot orange" />
                  Решение принято {formatDate(data.decidedAt)}
                </p>
              )}
              {data.grantedAt && (
                <p>
                  <span className="dot green" />
                  Доступ выдан {formatDate(data.grantedAt)}
                </p>
              )}
            </div>
            {profile.role === 'manager' &&
              data.status === 'pending' &&
              (rejecting ? (
                <form
                  onSubmit={(event) => {
                    event.preventDefault()
                    void decide('reject')
                  }}
                >
                  <label htmlFor="reason">Причина отказа для стажёра</label>
                  <textarea
                    id="reason"
                    value={reason}
                    onChange={(event) => setReason(event.target.value)}
                    placeholder="Что нужно уточнить или исправить?"
                    required
                    maxLength={2000}
                    autoFocus
                  />
                  <div className="dialog-actions">
                    <Button disabled={busy} onClick={() => setRejecting(false)}>
                      Назад
                    </Button>
                    <Button type="submit" variant="danger" disabled={busy || !reason.trim()}>
                      {busy ? 'Сохраняем…' : 'Отклонить заявку'}
                    </Button>
                  </div>
                </form>
              ) : (
                <div className="dialog-actions">
                  <Button variant="danger" disabled={busy} onClick={() => setRejecting(true)}>
                    Отклонить
                  </Button>
                  <Button variant="primary" disabled={busy} onClick={() => void decide('approve')}>
                    {busy ? 'Сохраняем…' : 'Одобрить'}
                  </Button>
                </div>
              ))}
            {profile.role === 'manager' && data.status === 'failed' && (
              <div className="dialog-actions">
                <Button variant="primary" disabled={busy} onClick={() => void decide('retry')}>
                  Повторить выдачу
                </Button>
              </div>
            )}
          </>
        )
      )}
      {error && <ErrorNotice message={error} retry={() => void refresh()} />}
      {actionError && <ErrorNotice message={actionError} />}
    </Modal>
  )
}
function RequestParameters({ request: r }: { request: AccessRequest }) {
  return (
    <dl>
      <dt>Сотрудник</dt>
      <dd>{employeeName(r.employeeId)}</dd>
      <dt>Ресурс</dt>
      <dd>{r.parameters.resourceName}</dd>
      <dt>Уровень</dt>
      <dd>{r.parameters.accessLevel}</dd>
      <dt>Для чего</dt>
      <dd>{r.parameters.purpose}</dd>
      <dt>Согласующий</dt>
      <dd>{r.parameters.approverName}</dd>
    </dl>
  )
}
