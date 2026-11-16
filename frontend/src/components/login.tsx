import { useState, type FormEvent } from 'react'
import { client, isDemo } from '../lib/client'
import type { LoginResult, Profile } from '../lib/types'
import { Button, ErrorNotice, Icon, Logo } from './ui'

type Registration = Extract<LoginResult, { status: 'registration_required' }>
export function Login({ onLogin }: { onLogin: (profile: Profile) => void }) {
  const [identifier, setIdentifier] = useState('')
  const [registration, setRegistration] = useState<Registration>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function login(value: string) {
    setBusy(true)
    setError('')
    try {
      const result = await client.login(value.trim())
      if (result.status === 'ready') onLogin(result.profile)
      else setRegistration(result)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось войти.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="login-shell">
      <header>
        <Logo />
        {isDemo && <span className="mode-label">Демо-просмотр</span>}
      </header>
      <main className="login-layout">
        <section className="login-intro">
          <span className="eyebrow">
            <Icon name="sparkle" />
            Рядом с первого дня
          </span>
          <h1>
            Новая команда.
            <br />
            Спокойный старт.
          </h1>
          <p>
            Знакомься с коллегами, получай доступы
            <br className="desktop-break" /> и двигайся к первой задаче вместе с AI-buddy.
          </p>
          <img
            src="/onboarding.svg"
            className="login-art"
            alt="Иллюстрация плана адаптации с выполненными шагами"
          />
          <div className="login-note">
            <span className="mini-dot" />
            Всё нужное для первых дней — в одном месте
          </div>
        </section>
        <section className="login-card panel">
          {registration ? (
            <RegistrationForm
              registration={registration}
              onLogin={onLogin}
              onBack={() => {
                setRegistration(undefined)
                setError('')
              }}
            />
          ) : (
            <>
              <span className="small muted">Добро пожаловать</span>
              <h2>Давай познакомимся</h2>
              <p className="muted">
                Введи демо-почту или ID сотрудника.
                <br />
                При первом входе выбери команду и наставника.
              </p>
              <form
                onSubmit={(event: FormEvent) => {
                  event.preventDefault()
                  void login(identifier)
                }}
              >
                <label htmlFor="identifier">Рабочая почта или ID</label>
                <input
                  id="identifier"
                  autoComplete="username"
                  autoFocus
                  placeholder="alina.search@example.test"
                  value={identifier}
                  onChange={(event) => setIdentifier(event.target.value)}
                  required
                  maxLength={254}
                  disabled={busy}
                />
                {error && <ErrorNotice message={error} />}
                <Button
                  type="submit"
                  variant="primary"
                  icon="arrow"
                  className="full-width"
                  disabled={busy || !identifier.trim()}
                >
                  {busy ? 'Входим…' : 'Продолжить'}
                </Button>
              </form>
              <div className="login-divider">
                <span>Попробовать демо</span>
              </div>
              <div className="demo-logins">
                <Button disabled={busy} onClick={() => void login('intern-alina')}>
                  <Icon name="people" />
                  Войти стажёром
                </Button>
                <Button disabled={busy} onClick={() => void login('manager-dmitry')}>
                  <Icon name="list" />
                  Войти руководителем
                </Button>
              </div>
              <p className="login-help">
                <button
                  className="text-link"
                  disabled={busy}
                  onClick={() => void login('intern-ivan')}
                >
                  Зарегистрироваться
                </button>
              </p>
            </>
          )}
        </section>
      </main>
      <footer className="login-footer">AI-buddy · С заботой о первых шагах</footer>
    </div>
  )
}
function RegistrationForm({
  registration: r,
  onLogin,
  onBack,
}: {
  registration: Registration
  onLogin: (profile: Profile) => void
  onBack: () => void
}) {
  const teams = r.teamOptions?.length ? r.teamOptions : [r.team]
  const [teamId, setTeamId] = useState(r.team.id)
  const team = teams.find((item) => item.id === teamId) || r.team
  const positions = team.positionOptions || r.positionOptions
  const [positionId, setPositionId] = useState(r.positionId)
  const [mentorId, setMentorId] = useState(r.team.mentors[0]?.id || '')
  const [date, setDate] = useState('')
  const [fullName, setFullName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      onLogin(
        await client.register(r.employeeId, {
          fullName: fullName.trim(),
          positionId,
          ...(r.role === 'intern' ? { teamId, mentorId, internshipStartDate: date } : {}),
        }),
      )
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось сохранить профиль.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <>
      <button className="text-link" onClick={onBack}>
        <Icon name="back" />
        Другой сотрудник
      </button>
      <h2>Регистрация</h2>
      <p className="muted">
        {r.role === 'intern'
          ? 'Выбери команду, должность и наставника — по ним подготовим твой план.'
          : `Ты руководитель команды «${r.team.name}». Укажи должность для профиля.`}
      </p>
      <form onSubmit={submit}>
        <label htmlFor="full-name">ФИО</label>
        <input
          id="full-name"
          autoComplete="name"
          placeholder="Иванов Иван Иванович"
          value={fullName}
          onChange={(event) => setFullName(event.target.value)}
          required
          minLength={3}
          maxLength={150}
          aria-describedby="full-name-hint"
        />
        <p id="full-name-hint" className="muted small">
          Фамилия, имя и отчество, если есть.
        </p>
        {r.role === 'intern' && (
          <>
            <label htmlFor="team">Команда</label>
            <select
              id="team"
              value={teamId}
              required
              onChange={(event) => {
                const next = teams.find((item) => item.id === event.target.value)!
                setTeamId(next.id)
                setMentorId(next.mentors[0]?.id || '')
                setPositionId((next.positionOptions || r.positionOptions)[0]?.id || '')
              }}
            >
              {teams.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.name}
                </option>
              ))}
            </select>
          </>
        )}
        <label htmlFor="position">Должность</label>
        <select
          id="position"
          value={positionId}
          onChange={(event) => setPositionId(event.target.value)}
          required
        >
          {positions.map((option) => (
            <option key={option.id} value={option.id}>
              {option.name}
            </option>
          ))}
        </select>
        {r.role === 'intern' && (
          <>
            <label htmlFor="mentor">Наставник</label>
            <select
              id="mentor"
              value={mentorId}
              onChange={(event) => setMentorId(event.target.value)}
              required
            >
              {team.mentors.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.firstName} {option.secondName}
                </option>
              ))}
            </select>
            <label htmlFor="start-date">Начало стажировки</label>
            <input
              id="start-date"
              type="date"
              value={date}
              onChange={(event) => setDate(event.target.value)}
              required
            />
          </>
        )}
        {error && <ErrorNotice message={error} />}
        <Button type="submit" variant="primary" className="full-width" disabled={busy}>
          {busy ? 'Сохраняем…' : 'Открыть мой план'}
        </Button>
      </form>
    </>
  )
}
