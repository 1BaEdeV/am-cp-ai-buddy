import { useCallback, useEffect, useState } from 'react'
import { client, isDemo } from './lib/client'
import type { OnboardingPlan, Profile, TaskId, WorkingAccesses } from './lib/types'
import { useResource } from './lib/use-resource'
import { Header, ProfileModal, Sidebar, type Screen } from './components/layout'
import { Login } from './components/login'
import { Plan, TaskPage } from './components/plan'
import { RequestModal, RequestsPage } from './components/requests'
import { Button, ErrorNotice, Loading, Modal } from './components/ui'

const screens: Screen[] = ['plan', 'requests', 'team-intro', 'working-accesses', 'first-task']
function readScreen(): Screen {
  const value = location.hash.slice(1).split('?')[0] as Screen
  return screens.includes(value) ? value : 'plan'
}
function readRequestId() {
  return new URLSearchParams(location.hash.split('?')[1] || '').get('request') || undefined
}
function fallbackPlan(accesses?: WorkingAccesses): OnboardingPlan {
  const done =
    !!accesses &&
    accesses.progress.total > 0 &&
    accesses.progress.completed === accesses.progress.total
  return {
    tasks: [
      {
        id: 'team-intro',
        title: 'Знакомство с командой',
        subtitle: 'Команда, руководитель и наставник',
        status: 'not_started',
      },
      {
        id: 'working-accesses',
        title: 'Рабочие доступы',
        subtitle: 'Ресурсы команды',
        status: done
          ? 'completed'
          : accesses?.progress.completed || accesses?.progress.inProgress
            ? 'in_progress'
            : 'not_started',
      },
      {
        id: 'first-task',
        title: 'Первая задача',
        subtitle: 'Учебная задача от руководителя',
        status: 'locked',
      },
    ],
    progress: { completed: Number(done), total: 3 },
    nextTaskId: 'team-intro',
  }
}
export default function App() {
  const [profile, setProfile] = useState<Profile>()
  return profile ? (
    <Workspace
      key={profile.employeeId}
      profile={profile}
      onLogin={setProfile}
      onLogout={() => {
        setProfile(undefined)
        history.replaceState(null, '', location.pathname + location.search)
      }}
    />
  ) : (
    <Login onLogin={setProfile} />
  )
}
function Workspace({
  profile,
  onLogin,
  onLogout,
}: {
  profile: Profile
  onLogin: (profile: Profile) => void
  onLogout: () => void
}) {
  const [screen, setScreen] = useState<Screen>(readScreen)
  const [modal, setModal] = useState<'profile' | 'meeting'>()
  const [requestId, setRequestId] = useState<string | undefined>(readRequestId)
  const [switchError, setSwitchError] = useState('')
  const manager = profile.role === 'manager'
  const loadAccesses = useCallback(
    () => (manager ? Promise.resolve(undefined) : client.accesses(profile.employeeId)),
    [profile.employeeId, manager],
  )
  const loadPlan = useCallback(
    () => (manager ? Promise.resolve(undefined) : client.plan(profile.employeeId)),
    [profile.employeeId, manager],
  )
  const loadRequests = useCallback(() => client.requests(profile.employeeId), [profile.employeeId])
  const accesses = useResource(loadAccesses, 2500)
  const planData = useResource(loadPlan, 5000)
  const requests = useResource(loadRequests, 2500)
  const plan = planData.data || fallbackPlan(accesses.data)
  const refresh = () => {
    void accesses.refresh()
    void planData.refresh()
    void requests.refresh()
  }
  useEffect(() => {
    const change = () => {
      setScreen(readScreen())
      setRequestId(readRequestId())
    }
    window.addEventListener('hashchange', change)
    return () => window.removeEventListener('hashchange', change)
  }, [])
  function navigate(next: Screen) {
    location.hash = next
    window.scrollTo({ top: 0 })
  }
  function openRequest(id: string) {
    location.hash = `${screen}?${new URLSearchParams({ request: id })}`
  }
  function closeRequest() {
    setRequestId(undefined)
    if (readRequestId()) location.hash = screen
  }
  async function switchRole(id: string) {
    setSwitchError('')
    try {
      const result = await client.login(id)
      if (result.status === 'ready') onLogin(result.profile)
    } catch (error) {
      setSwitchError(error instanceof Error ? error.message : 'Не удалось сменить сотрудника.')
    }
  }
  const task = !['plan', 'requests'].includes(screen) ? (screen as TaskId) : undefined
  return (
    <>
      <a
        className="skip-link"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault()
          document.getElementById('main-content')?.focus()
        }}
      >
        К содержимому
      </a>
      <Header
        profile={profile}
        screen={screen}
        navigate={navigate}
        openProfile={() => setModal('profile')}
        onRole={(id) => void switchRole(id)}
      />
      <main className="shell" id="main-content" tabIndex={-1}>
        {switchError && <ErrorNotice message={switchError} />}
        {manager ? (
          <div className="manager">
            <RequestsPage profile={profile} openRequest={openRequest} />
          </div>
        ) : (
          <div className="layout">
            <div className="main">
              {accesses.error && (
                <ErrorNotice message={accesses.error} retry={() => void accesses.refresh()} />
              )}
              {screen === 'requests' ? (
                <RequestsPage profile={profile} openRequest={openRequest} />
              ) : accesses.loading ? (
                <section className="panel">
                  <Loading label="Готовим твой план…" />
                </section>
              ) : (
                <>
                  {planData.error && (
                    <div className="plan-unavailable">
                      <p>
                        Общий план пока недоступен. Рабочие доступы и заявки можно открыть отдельно.
                      </p>
                      <button className="text-link" onClick={() => void planData.refresh()}>
                        Обновить план
                      </button>
                    </div>
                  )}
                  {task ? (
                    <TaskPage
                      key={task}
                      taskId={task}
                      profile={profile}
                      plan={plan}
                      accesses={accesses.data}
                      requests={requests.data?.items || []}
                      navigate={navigate}
                      onChange={refresh}
                      openRequest={openRequest}
                    />
                  ) : (
                    <Plan
                      profile={profile}
                      plan={plan}
                      accesses={accesses.data}
                      navigate={navigate}
                    />
                  )}
                </>
              )}
            </div>
            <Sidebar
              plan={plan}
              accesses={accesses.data}
              counts={requests.data?.counts}
              navigate={navigate}
              openMeeting={() => setModal('meeting')}
            />
          </div>
        )}
        <footer className="foot">
          <span>AI-buddy · Твой помощник в новой команде</span>
          <span>{isDemo ? 'Демо-просмотр · вымышленные данные' : profile.team.name}</span>
        </footer>
      </main>
      {modal === 'profile' && (
        <ProfileModal profile={profile} onClose={() => setModal(undefined)} onLogout={onLogout} />
      )}
      {modal === 'meeting' && (
        <Modal title="Встреча с наставником" onClose={() => setModal(undefined)}>
          {plan.upcomingMeeting ? (
            <dl>
              <dt>Наставник</dt>
              <dd>{plan.upcomingMeeting.mentorName}</dd>
              <dt>Когда</dt>
              <dd>
                {new Date(plan.upcomingMeeting.startsAt).toLocaleString('ru', {
                  timeZone: 'Europe/Moscow',
                  day: 'numeric',
                  month: 'long',
                  hour: '2-digit',
                  minute: '2-digit',
                })}{' '}
                МСК
              </dd>
              <dt>Формат</dt>
              <dd>{plan.upcomingMeeting.format}</dd>
              <dt>Повестка</dt>
              <dd>{plan.upcomingMeeting.agenda}</dd>
            </dl>
          ) : (
            <p className="muted">Ближайшая встреча пока не назначена.</p>
          )}
          <div className="dialog-actions">
            <Button onClick={() => setModal(undefined)}>Понятно</Button>
          </div>
        </Modal>
      )}
      {requestId && (
        <RequestModal
          key={requestId}
          profile={profile}
          requestId={requestId}
          onClose={closeRequest}
          onChange={refresh}
        />
      )}
    </>
  )
}
