import { useCallback, useState } from 'react'
import { client } from '../lib/client'
import type { AccessRequest, OnboardingPlan, Profile, TaskId, WorkingAccesses } from '../lib/types'
import { useResource } from '../lib/use-resource'
import { Chat } from './chat'
import type { Screen } from './layout'
import {
  Avatar,
  Badge,
  Button,
  Empty,
  ErrorNotice,
  Icon,
  Loading,
  Status,
  Tile,
  type IconName,
} from './ui'

const taskIcons: Record<TaskId, IconName> = {
  'team-intro': 'people',
  'working-accesses': 'key',
  'first-task': 'flag',
}
export function Plan({
  profile,
  plan,
  accesses,
  navigate,
}: {
  profile: Profile
  plan: OnboardingPlan
  accesses?: WorkingAccesses
  navigate: (screen: Screen) => void
}) {
  return (
    <>
      <section className="hero">
        <div className="hero-copy">
          <span className="eyebrow">
            <Icon name="sparkle" />
            Первые шаги · команда «{profile.team.name}»
          </span>
          <h1>Привет, {profile.firstName}!</h1>
          <p>
            Познакомься с командой, получи доступы
            <br className="desktop-break" /> и подготовься к первой задаче.
          </p>
          <Button variant="primary" icon="arrow" onClick={() => navigate(plan.nextTaskId)}>
            Продолжить адаптацию
          </Button>
        </div>
        <img className="hero-art" src="/onboarding.svg" alt="" />
      </section>
      <div className="section-head">
        <div>
          <h2>План адаптации</h2>
          <p className="muted small">По одному шагу — к уверенному старту</p>
        </div>
        <span className="small muted">{plan.progress.completed} из 3 этапов</span>
      </div>
      <div className="task-list">
        {plan.tasks.map((task, index) => (
          <button
            key={task.id}
            className={`task-row ${task.id === plan.nextTaskId ? 'active' : ''}`}
            onClick={() => navigate(task.id)}
          >
            <Tile name={taskIcons[task.id]} />
            <span className="task-text">
              <span className="task-step">ШАГ {index + 1}</span>
              <strong>{task.title}</strong>
              <span>
                {task.id === 'working-accesses' && accesses
                  ? `${accesses.progress.completed} из ${accesses.progress.total} доступов выдано · Ресурсы команды`
                  : task.subtitle}
              </span>
            </span>
            <Status value={task.status} />
            <Icon name="chevron" className="chevron" />
          </button>
        ))}
      </div>
      <div className="plan-footnote">
        <Icon name="sparkle" />
        <p>Нужна подсказка? Внутри каждого шага есть чат с AI-buddy.</p>
      </div>
    </>
  )
}
export function TaskPage({
  taskId,
  profile,
  plan,
  accesses,
  requests,
  navigate,
  onChange,
  openRequest,
}: {
  taskId: TaskId
  profile: Profile
  plan: OnboardingPlan
  accesses?: WorkingAccesses
  requests: AccessRequest[]
  navigate: (screen: Screen) => void
  onChange: () => void
  openRequest: (id: string) => void
}) {
  const [tab, setTab] = useState<'overview' | 'chat'>('overview')
  const task = plan.tasks.find((t) => t.id === taskId)!
  return (
    <div className="detail">
      <div className="detail-head">
        <button className="text-link" onClick={() => navigate('plan')}>
          <Icon name="back" />К плану адаптации
        </button>
        <div className="title-row">
          <h1>{task.title}</h1>
          <Status value={task.status} />
        </div>
        <p className="muted">
          {taskId === 'team-intro'
            ? 'Узнай, чем занимается команда и к кому обращаться.'
            : taskId === 'working-accesses'
              ? 'Всё необходимое для работы — с понятным статусом по каждому ресурсу.'
              : 'Первый небольшой шаг в большую работу.'}
        </p>
      </div>
      {taskId !== 'first-task' && (
        <div className="segments task-tabs" role="tablist" aria-label="Раздел задания">
          <button
            role="tab"
            id="overview-tab"
            aria-selected={tab === 'overview'}
            aria-controls="task-content"
            aria-pressed={tab === 'overview'}
            onClick={() => setTab('overview')}
          >
            Задание
          </button>
          <button
            role="tab"
            id="chat-tab"
            aria-selected={tab === 'chat'}
            aria-controls="task-content"
            aria-pressed={tab === 'chat'}
            onClick={() => setTab('chat')}
          >
            <Icon name="sparkle" />
            Чат с AI-buddy
          </button>
        </div>
      )}
      <div
        id="task-content"
        role={taskId === 'first-task' ? undefined : 'tabpanel'}
        aria-labelledby={tab === 'overview' ? 'overview-tab' : 'chat-tab'}
      >
        {tab === 'chat' ? (
          <Chat
            profile={profile}
            taskId={taskId}
            resources={accesses?.resources || []}
            requests={requests}
            onChange={onChange}
            openRequest={openRequest}
          />
        ) : taskId === 'team-intro' ? (
          <Intro
            profile={profile}
            completed={task.status === 'completed'}
            onChange={onChange}
            openChat={() => setTab('chat')}
          />
        ) : taskId === 'working-accesses' ? (
          <Resources
            accesses={accesses}
            openChat={() => setTab('chat')}
            openRequest={openRequest}
          />
        ) : (
          <section className="panel first-task">
            <Empty
              icon={task.status === 'coming_soon' ? 'flag' : 'lock'}
              title={
                task.status === 'coming_soon'
                  ? 'Здесь появится первая задача'
                  : 'Сначала — немного подготовки'
              }
              text={
                task.status === 'coming_soon'
                  ? 'Ты готов к следующему шагу. Руководитель добавит описание и материалы для учебной задачи.'
                  : 'Познакомься с командой и получи обязательные доступы. После этого здесь появится твоя первая учебная задача.'
              }
            >
              {task.status === 'coming_soon' ? (
                <Badge tone="blue">Скоро</Badge>
              ) : (
                <Button variant="primary" onClick={() => navigate(plan.nextTaskId)}>
                  Перейти к текущему шагу
                </Button>
              )}
            </Empty>
          </section>
        )}
      </div>
    </div>
  )
}
function Intro({
  profile,
  completed,
  onChange,
  openChat,
}: {
  profile: Profile
  completed: boolean
  onChange: () => void
  openChat: () => void
}) {
  const load = useCallback(() => client.intro(profile.employeeId), [profile.employeeId])
  const { data, loading, error, refresh } = useResource(load)
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState('')
  async function complete() {
    setBusy(true)
    setActionError('')
    try {
      await client.completeIntro(profile.employeeId)
      onChange()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось отметить знакомство.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="panel intro-panel">
      {loading ? (
        <Loading />
      ) : data ? (
        <>
          <span className="eyebrow">Рады видеть тебя с нами</span>
          <h2>Команда «{data.team.name}»</h2>
          <p className="team-intro">{data.description}</p>
          <div className="people-grid">
            <div className="person">
              <Avatar name={data.manager.fullName} tone="blue" />
              <div>
                <strong>{data.manager.fullName}</strong>
                <p>{data.manager.position}</p>
                {data.manager.email && <p>{data.manager.email}</p>}
                {data.manager.about && <p className="person-about">{data.manager.about}</p>}
              </div>
            </div>
            <div className="person">
              <Avatar name={data.mentor.fullName} />
              <div>
                <strong>{data.mentor.fullName}</strong>
                <p>{data.mentor.position}</p>
                {data.mentor.email && <p>{data.mentor.email}</p>}
                {data.mentor.about && <p className="person-about">{data.mentor.about}</p>}
              </div>
            </div>
          </div>
          <div className="intro-tip">
            <Icon name="people" />
            <p>
              Поздоровайся с коллегами и обсуди с наставником план первых дней. Когда познакомишься,
              отметь этот шаг.
            </p>
          </div>
          <div className="detail-actions">
            {completed ? (
              <Badge tone="green">
                <Icon name="check" />
                Знакомство отмечено
              </Badge>
            ) : (
              <Button variant="primary" disabled={busy} onClick={() => void complete()}>
                {busy ? 'Сохраняем…' : 'Отметить знакомство'}
              </Button>
            )}
            <Button icon="sparkle" onClick={openChat}>
              Спросить AI-buddy
            </Button>
          </div>
        </>
      ) : null}
      {error && <ErrorNotice message={error} retry={() => void refresh()} />}
      {actionError && <ErrorNotice message={actionError} />}
    </section>
  )
}
function Resources({
  accesses,
  openChat,
  openRequest,
}: {
  accesses?: WorkingAccesses
  openChat: () => void
  openRequest: (id: string) => void
}) {
  return (
    <section className="panel resources-panel">
      <div className="title-row">
        <h2>Ресурсы команды</h2>
        {accesses && (
          <Badge tone={accesses.progress.completed === accesses.progress.total ? 'green' : 'blue'}>
            {accesses.progress.completed} из {accesses.progress.total} выдано
          </Badge>
        )}
      </div>
      <p className="muted small section-description">
        Уровни и согласующие определены для твоей роли.
      </p>
      {accesses ? (
        <div className="resource-list">
          {accesses.resources.map((resource) => (
            <div className="resource" key={resource.id}>
              <Tile name="key" />
              <div className="resource-text">
                <strong>{resource.name}</strong>
                <p>
                  <span className="resource-level">{resource.accessLevel}</span>
                  {resource.purpose}
                </p>
              </div>
              <div className="resource-actions">
                <Status value={resource.status} />
                {resource.request ? (
                  <button className="text-link" onClick={() => openRequest(resource.request!.id)}>
                    № {resource.request.number}
                    <Icon name="chevron" />
                  </button>
                ) : (
                  resource.canRequest && (
                    <button className="text-link" onClick={openChat}>
                      Обсудить с AI-buddy
                      <Icon name="chevron" />
                    </button>
                  )
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <Loading />
      )}
      <div className="resource-callout">
        <Tile name="sparkle" tone="people" />
        <div>
          <strong>Нужен доступ? Начни с AI-buddy</strong>
          <p>
            Помощник подготовит один ресурс и покажет параметры. Отправка — только после твоего
            подтверждения.
          </p>
        </div>
        <Button variant="primary" onClick={openChat}>
          Открыть чат
        </Button>
      </div>
    </section>
  )
}
