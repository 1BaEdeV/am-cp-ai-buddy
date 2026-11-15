import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { client, isDemo } from '../lib/client'
import type { AccessDraft, AccessRequest, Profile, RequiredResource, TaskId } from '../lib/types'
import { useResource } from '../lib/use-resource'
import { Button, ErrorNotice, Icon, Loading, Status, Tile } from './ui'

export function Chat({
  profile,
  taskId,
  resources,
  requests,
  onChange,
  openRequest,
}: {
  profile: Profile
  taskId: TaskId
  resources: RequiredResource[]
  requests: AccessRequest[]
  onChange: () => void
  openRequest: (id: string) => void
}) {
  const load = useCallback(
    () => client.chat(profile.employeeId, taskId),
    [profile.employeeId, taskId],
  )
  const { data, error, loading, refresh } = useResource(load, 2000)
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState('')
  const messagesEnd = useRef<HTMLDivElement>(null)
  const runBusy = data?.latestRun?.status === 'queued' || data?.latestRun?.status === 'running'
  const busy = sending || runBusy
  useEffect(() => {
    messagesEnd.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }, [data?.messages.length, busy])
  async function send(question: string, resourceId?: string) {
    const value = question.trim()
    if (!value || busy) return
    setSending(true)
    setSendError('')
    try {
      await client.sendMessage(profile.employeeId, taskId, value, resourceId)
      setText('')
      await refresh()
      onChange()
    } catch (error) {
      setSendError(error instanceof Error ? error.message : 'Не удалось отправить сообщение.')
    } finally {
      setSending(false)
    }
  }
  function submit(event: FormEvent) {
    event.preventDefault()
    void send(text)
  }
  const hello =
    taskId === 'team-intro'
      ? `Привет, ${profile.firstName}! Давай познакомимся с командой. Подскажу, кто твой наставник и с чего начать.`
      : `Помогу разобраться с рабочими доступами. Выбери один ресурс — покажу уровень, назначение и согласующего перед отправкой заявки.`
  return (
    <section className="panel chat">
      <div className="chat-head">
        <Tile name="sparkle" tone="people" />
        <div>
          <h3>AI-buddy</h3>
          <span className="small muted">Рядом на каждом шаге</span>
        </div>
        <span className="chat-label">{isDemo ? 'Демо-диалог' : 'Чат задания'}</span>
      </div>
      {loading && !data ? (
        <Loading />
      ) : error ? (
        <div className="chat-unavailable">
          <Tile name="sparkle" tone="people" />
          <h3>Чат пока недоступен</h3>
          <p className="muted">
            Попробуй обновить его позже. Существующие заявки доступны в разделе «Мои заявки».
          </p>
          <ErrorNotice message={error} retry={() => void refresh()} />
        </div>
      ) : (
        <>
          <div className="messages" role="log" aria-label="Переписка с AI-buddy">
            <div className="message assistant">
              <span className="message-author">AI-buddy</span>
              <p>{hello}</p>
            </div>
            {data?.messages.map((message) => (
              <div key={message.id} className={`message ${message.role}`}>
                <span className="message-author">
                  {message.role === 'user' ? 'Ты' : 'AI-buddy'}
                </span>
                <p>{message.text}</p>
                {message.sourceLabel && (
                  <span className="source-note">
                    <Icon name="check" />
                    {message.sourceLabel}
                  </span>
                )}
                {message.draft && (
                  <DraftCard
                    profile={profile}
                    draft={message.draft}
                    existing={requests.find((r) => r.draftId === message.draft!.id)}
                    openRequest={openRequest}
                    onChange={onChange}
                  />
                )}
                {message.request && (
                  <button
                    className="text-link inline-request"
                    onClick={() => openRequest(message.request!.id)}
                  >
                    Заявка № {message.request.number}
                    <Icon name="chevron" />
                  </button>
                )}
              </div>
            ))}
            {busy && (
              <div className="message assistant typing" role="status">
                <span />
                <span />
                <span />
                <span className="sr-only">AI-buddy готовит ответ</span>
              </div>
            )}
            <div ref={messagesEnd} />
          </div>
          <div className="chat-footer">
            {taskId === 'working-accesses' ? (
              <div className="chips">
                {resources
                  .filter((r) => r.canRequest)
                  .map((resource) => (
                    <button
                      key={resource.id}
                      className="chip"
                      disabled={busy}
                      onClick={() => void send(`Запроси доступ к ${resource.name}`, resource.id)}
                    >
                      <Icon name="key" />
                      Запросить {resource.service === 'gitlab' ? 'GitLab' : resource.name}
                    </button>
                  ))}
              </div>
            ) : (
              <div className="chips">
                {['Кто мой наставник?', 'Чем занимается команда?'].map((question) => (
                  <button
                    key={question}
                    className="chip"
                    disabled={busy}
                    onClick={() => void send(question)}
                  >
                    {question}
                  </button>
                ))}
              </div>
            )}
            {(sendError || data?.latestRun?.status === 'failed') && (
              <ErrorNotice
                message={
                  sendError ||
                  data?.latestRun?.error ||
                  'Не удалось подготовить ответ. Попробуй отправить вопрос ещё раз.'
                }
              />
            )}
            <form className="send" onSubmit={submit}>
              <input
                aria-label="Вопрос AI-buddy"
                placeholder="Спроси по этому заданию"
                value={text}
                onChange={(event) => setText(event.target.value)}
                maxLength={450}
                required
                disabled={busy}
              />
              <button
                className="btn primary send-button"
                type="submit"
                aria-label="Отправить сообщение"
                disabled={busy || !text.trim()}
              >
                <Icon name="send" />
              </button>
            </form>
            <p className="chat-disclaimer">
              {isDemo
                ? 'Демо-ответы по данным команды. Подтверждение заявки — всегда за тобой.'
                : 'Проверь параметры доступа перед подтверждением.'}
            </p>
          </div>
        </>
      )}
    </section>
  )
}
function DraftCard({
  profile,
  draft,
  existing,
  onChange,
  openRequest,
}: {
  profile: Profile
  draft: AccessDraft
  existing?: AccessRequest
  onChange: () => void
  openRequest: (id: string) => void
}) {
  const [result, setResult] = useState<AccessRequest>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const submitted = existing || result
  const [expired, setExpired] = useState(() => Date.parse(draft.expiresAt) <= Date.now())
  useEffect(() => {
    const timer = window.setTimeout(
      () => setExpired(true),
      Math.max(0, Date.parse(draft.expiresAt) - Date.now()),
    )
    return () => clearTimeout(timer)
  }, [draft.expiresAt])
  async function confirm() {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      setResult(await client.confirm(profile.employeeId, draft.id))
      onChange()
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось отправить заявку.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="draft-card">
      <div className="draft-title">
        <Tile name="key" />
        <div>
          <span className="small muted">
            {submitted ? 'Заявка отправлена' : 'Черновик одного доступа'}
          </span>
          <h3>{draft.parameters.resourceName}</h3>
        </div>
      </div>
      <dl>
        <dt>Уровень</dt>
        <dd>{draft.parameters.accessLevel}</dd>
        <dt>Для чего</dt>
        <dd>{draft.parameters.purpose}</dd>
        <dt>Согласующий</dt>
        <dd>{draft.parameters.approverName}</dd>
      </dl>
      {submitted ? (
        <>
          <Status value={submitted.status} />
          <button className="text-link" onClick={() => openRequest(submitted.id)}>
            Открыть заявку № {submitted.number}
            <Icon name="chevron" />
          </button>
        </>
      ) : (
        <>
          <p className="small muted">
            {expired
              ? 'Срок черновика истёк. Попроси подготовить новый.'
              : 'После подтверждения заявка отправится руководителю.'}
          </p>
          <Button
            variant="primary"
            className="full-width"
            onClick={() => void confirm()}
            disabled={busy || expired}
          >
            {busy ? 'Отправляем…' : 'Подтвердить отправку'}
          </Button>
        </>
      )}
      {error && <ErrorNotice message={error} />}
    </div>
  )
}
