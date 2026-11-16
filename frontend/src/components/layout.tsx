import type { OnboardingPlan, Profile, RequestCounts, TaskId, WorkingAccesses } from '../lib/types'
import { isDemo } from '../lib/client'
import { Avatar, Button, Icon, Logo, Modal } from './ui'

export type Screen = 'plan' | 'requests' | TaskId
export function Header({
  profile,
  screen,
  navigate,
  openProfile,
  onRole,
}: {
  profile: Profile
  screen: Screen
  navigate: (screen: Screen) => void
  openProfile: () => void
  onRole: (id: string) => void
}) {
  return (
    <header className="top">
      <div className="top-inner">
        <button onClick={() => navigate('plan')} aria-label="AI-buddy, на главную">
          <Logo />
        </button>
        <nav className="nav" aria-label="Основная навигация">
          {profile.role === 'intern' ? (
            <>
              <button
                className={screen !== 'requests' ? 'current' : ''}
                aria-current={screen !== 'requests' ? 'page' : undefined}
                onClick={() => navigate('plan')}
              >
                Мой план
              </button>
              <button
                className={screen === 'requests' ? 'current' : ''}
                aria-current={screen === 'requests' ? 'page' : undefined}
                onClick={() => navigate('requests')}
              >
                Мои заявки
              </button>
            </>
          ) : (
            <button className="current" aria-current="page" onClick={() => navigate('requests')}>
              Заявки команды
            </button>
          )}
        </nav>
        <div className="top-actions">
          {isDemo && (
            <div className="demo-switch">
              <span className="role-label">ДЕМО</span>
              <div className="segments" aria-label="Роль в демо">
                <button
                  aria-pressed={profile.role === 'intern'}
                  onClick={() => onRole('intern-alina')}
                >
                  Стажёр
                </button>
                <button
                  aria-pressed={profile.role === 'manager'}
                  onClick={() => onRole('manager-dmitry')}
                >
                  Руководитель
                </button>
              </div>
            </div>
          )}
          <button className="profile-trigger" onClick={openProfile} aria-label="Открыть профиль">
            <Avatar
              name={`${profile.firstName} ${profile.secondName}`}
              tone={profile.role === 'manager' ? 'green' : ''}
            />
          </button>
        </div>
      </div>
    </header>
  )
}
export function Sidebar({
  plan,
  accesses,
  counts,
  navigate,
  openMeeting,
}: {
  plan: OnboardingPlan
  accesses?: WorkingAccesses
  counts?: RequestCounts
  navigate: (screen: Screen) => void
  openMeeting: () => void
}) {
  return (
    <aside className="sidebar" aria-label="Сводка адаптации">
      <section className="panel">
        <div className="side-title">
          <h3>Путь адаптации</h3>
          <Icon name="route" />
        </div>
        <div className="progress-summary">
          <strong>
            {plan.progress.completed}
            <span className="muted"> из 3 этапов</span>
          </strong>
          <span className="small muted">Твой темп</span>
        </div>
        <div className="progress" aria-hidden="true">
          {plan.tasks.map((task) => (
            <span
              key={task.id}
              className={
                task.status === 'completed' ? 'done' : task.id === plan.nextTaskId ? 'active' : ''
              }
            />
          ))}
        </div>
        <ol className="steps">
          {plan.tasks.map((task, index) => (
            <li key={task.id}>
              <button className="step" onClick={() => navigate(task.id)}>
                <span
                  className={`step-dot ${task.status === 'completed' ? 'done' : task.id === plan.nextTaskId ? 'active' : ''}`}
                >
                  {task.status === 'completed' ? <Icon name="check" /> : index + 1}
                </span>
                <span>{task.id === 'team-intro' ? 'Знакомство' : task.title}</span>
                {task.id === 'working-accesses' && accesses && (
                  <small className="muted step-count">
                    {accesses.progress.completed}/{accesses.progress.total}
                  </small>
                )}
              </button>
            </li>
          ))}
        </ol>
      </section>
      <section className="panel">
        <div className="side-title">
          <h3>Встречи с наставником</h3>
          <Icon name="calendar" />
        </div>
        {plan.upcomingMeeting ? (
          <>
            <div className="meeting">
              <span className="date">
                <b>
                  {new Date(plan.upcomingMeeting.startsAt).toLocaleDateString('ru', {
                    timeZone: 'Europe/Moscow',
                    day: 'numeric',
                  })}
                </b>
                <small>
                  {new Date(plan.upcomingMeeting.startsAt).toLocaleDateString('ru', {
                    timeZone: 'Europe/Moscow',
                    month: 'short',
                  })}
                </small>
              </span>
              <div>
                <strong>{plan.upcomingMeeting.title}</strong>
                <p>
                  {new Date(plan.upcomingMeeting.startsAt).toLocaleTimeString('ru', {
                    timeZone: 'Europe/Moscow',
                    hour: '2-digit',
                    minute: '2-digit',
                  })}{' '}
                  · {plan.upcomingMeeting.format}
                </p>
              </div>
            </div>
            <button className="text-link" onClick={openMeeting}>
              Подробнее о встрече
              <Icon name="chevron" />
            </button>
          </>
        ) : (
          <div className="meeting-placeholder">
            <span className="mini-calendar">
              <Icon name="calendar" />
            </span>
            <p>
              Здесь появится твоя
              <br />
              ближайшая встреча
            </p>
          </div>
        )}
      </section>
      <section className="panel">
        <div className="side-title">
          <h3>Мои заявки</h3>
          <Icon name="list" />
        </div>
        <div className="request-mini">
          <span>
            <i className="dot orange" />В работе
          </span>
          <strong>{counts?.active ?? '—'}</strong>
        </div>
        <div className="request-mini">
          <span>
            <i className="dot green" />
            Обработаны
          </span>
          <strong>{counts?.completed ?? '—'}</strong>
        </div>
        <button className="text-link" onClick={() => navigate('requests')}>
          Все заявки
          <Icon name="chevron" />
        </button>
      </section>
      <div className="sidebar-hint">
        <Icon name="sparkle" />
        <p>
          Не нужно разбираться во всём сразу.
          <br />
          AI-buddy поможет с каждым шагом.
        </p>
      </div>
    </aside>
  )
}
export function ProfileModal({
  profile,
  onClose,
  onLogout,
}: {
  profile: Profile
  onClose: () => void
  onLogout: () => void
}) {
  return (
    <Modal title="Мой профиль" onClose={onClose}>
      <div className="profile-heading">
        <Avatar name={`${profile.firstName} ${profile.secondName}`} />
        <div>
          <h3>
            {[profile.firstName, profile.middleName, profile.secondName].filter(Boolean).join(' ')}
          </h3>
          <p className="muted">{profile.position}</p>
        </div>
      </div>
      <dl>
        <dt>Роль</dt>
        <dd>{profile.role === 'intern' ? 'Стажёр' : 'Руководитель'}</dd>
        <dt>Команда</dt>
        <dd>{profile.team.name}</dd>
        <dt>Рабочая почта</dt>
        <dd>{profile.email}</dd>
        {profile.mentor && (
          <>
            <dt>Наставник</dt>
            <dd>
              {profile.mentor.firstName} {profile.mentor.secondName}
            </dd>
          </>
        )}
        {profile.internshipStartDate && (
          <>
            <dt>Начало стажировки</dt>
            <dd>{new Date(`${profile.internshipStartDate}T12:00:00`).toLocaleDateString('ru')}</dd>
          </>
        )}
      </dl>
      <div className="dialog-actions">
        <Button onClick={onLogout} icon="logout">
          Выйти
        </Button>
        <Button variant="primary" onClick={onClose}>
          Понятно
        </Button>
      </div>
    </Modal>
  )
}
