import { APIError } from './api'
import type {
  AccessDraft,
  AccessRequest,
  Client,
  LoginResult,
  Profile,
  RequestCounts,
  RequestGroup,
  RequiredResource,
  TaskChat,
  WorkingAccesses,
} from './types'

const key = 'ai-buddy-demo-v1'
const copy = <T>(value: T): T => structuredClone(value)
const now = () => new Date().toISOString()
const active = (status: string) => ['pending', 'issuing', 'failed'].includes(status)
const mentor = { id: 'mentor-katya', firstName: 'Катя', secondName: 'Волкова' }
const profiles: Record<string, Profile> = {
  'intern-alina': {
    id: 'profile-intern-alina',
    employeeId: 'intern-alina',
    firstName: 'Алина',
    secondName: 'Смирнова',
    email: 'alina.search@example.test',
    role: 'intern',
    position: 'Backend-разработчик Go',
    team: { id: 'team-search', name: 'Поиск' },
    mentor,
    internshipStartDate: '2026-11-10',
  },
  'manager-dmitry': {
    id: 'profile-manager-dmitry',
    employeeId: 'manager-dmitry',
    firstName: 'Дмитрий',
    secondName: 'Орлов',
    email: 'dmitry.search@example.test',
    role: 'manager',
    position: 'Руководитель команды',
    team: { id: 'team-search', name: 'Поиск' },
  },
  'manager-anna': {
    id: 'profile-manager-anna',
    employeeId: 'manager-anna',
    firstName: 'Анна',
    secondName: 'Соколова',
    email: 'anna.web@example.test',
    role: 'manager',
    position: 'Руководитель команды',
    team: { id: 'team-web', name: 'Веб-интерфейсы' },
  },
}
const initialResources: RequiredResource[] = [
  {
    id: 'team-wiki',
    service: 'wiki',
    name: 'Вики команды',
    accessLevel: 'read',
    purpose: 'Изучение инструкций и процессов адаптации.',
    status: 'granted',
    canRequest: false,
  },
  {
    id: 'youtrack-search',
    service: 'youtrack',
    name: 'YouTrack',
    accessLevel: 'member',
    purpose: 'Работа с учебными задачами.',
    status: 'granted',
    canRequest: false,
  },
  {
    id: 'gitlab-search-api',
    service: 'gitlab',
    name: 'GitLab · search-api',
    accessLevel: 'Developer',
    purpose: 'Работа с кодом учебного поискового сервиса.',
    status: 'requestable',
    canRequest: true,
  },
  {
    id: 'grafana-search',
    service: 'grafana',
    name: 'Grafana · Поиск',
    accessLevel: 'Viewer',
    purpose: 'Просмотр метрик учебного окружения.',
    status: 'requestable',
    canRequest: true,
  },
]
const webResources: RequiredResource[] = [
  ...initialResources.slice(0, 2),
  {
    id: 'gitlab-onboarding',
    service: 'gitlab',
    name: 'GitLab · onboarding',
    accessLevel: 'Developer',
    purpose: 'Разработка учебного интерфейса.',
    status: 'requestable',
    canRequest: true,
  },
  {
    id: 'figma-design',
    service: 'figma',
    name: 'Figma · макеты',
    accessLevel: 'Viewer',
    purpose: 'Просмотр макетов и параметров компонентов.',
    status: 'requestable',
    canRequest: true,
  },
]
interface State {
  version: 1
  profiles: Record<string, Profile>
  intro: Record<string, boolean>
  requests: AccessRequest[]
  drafts: Record<string, AccessDraft & { employeeId: string }>
  chats: Record<string, TaskChat>
  nextNumber: number
}
const initial = (): State => ({
  version: 1,
  profiles: copy(profiles),
  intro: {},
  requests: [],
  drafts: {},
  chats: {},
  nextNumber: 1040,
})
function restore(): State {
  try {
    const saved = JSON.parse(localStorage.getItem(key) || 'null')
    if (
      saved?.version === 1 &&
      Array.isArray(saved.requests) &&
      saved.profiles &&
      saved.drafts &&
      saved.chats &&
      saved.intro &&
      Number.isInteger(saved.nextNumber)
    )
      return saved
  } catch {
    /* Приватный режим или старые данные: начинаем новое демо. */
  }
  return initial()
}
let state = restore()
function save() {
  try {
    localStorage.setItem(key, JSON.stringify(state))
  } catch {
    /* Демо продолжает работать в памяти при недоступном хранилище. */
  }
}
export function resetDemo() {
  state = initial()
  save()
}
function employee(id: string) {
  const p = state.profiles[id]
  if (!p) throw new APIError('Сотрудник не найден. Войди заново.', 401, 'demo_identity_required')
  return p
}
function intern(id: string) {
  const p = employee(id)
  if (p.role !== 'intern') throw new APIError('Этот раздел доступен стажёру.', 403, 'forbidden')
  return p
}
function visible(id: string, request: AccessRequest) {
  const p = employee(id)
  return p.role === 'manager' ? request.approverId === id : request.employeeId === id
}
function findRequest(id: string, requestId: string) {
  const r = state.requests.find((r) => r.id === requestId && visible(id, r))
  if (!r) throw new APIError('Заявка не найдена.', 404, 'request_not_found')
  return r
}
function settle() {
  let changed = false
  for (const r of state.requests)
    if (r.status === 'issuing' && Date.now() - Date.parse(r.updatedAt) > 900) {
      r.status = 'granted'
      r.updatedAt = r.grantedAt = now()
      changed = true
    }
  if (changed) save()
}
function card(id: string): WorkingAccesses {
  const actor = intern(id)
  settle()
  const resources = (actor.team.id === 'team-web' ? webResources : initialResources).map(
    (resource) => {
      const r = state.requests.find(
        (r) =>
          r.employeeId === id &&
          r.resourceId === resource.id &&
          (active(r.status) || r.status === 'granted'),
      )
      if (r?.status === 'granted' || (id === 'intern-alina' && resource.status === 'granted'))
        return { ...resource, status: 'granted' as const, canRequest: false }
      if (r)
        return {
          ...resource,
          status: r.status as 'pending' | 'issuing' | 'failed',
          canRequest: false,
          request: { id: r.id, number: r.number, status: r.status },
        }
      return { ...resource, status: 'requestable' as const, canRequest: true }
    },
  )
  return {
    resources,
    progress: {
      completed: resources.filter((r) => r.status === 'granted').length,
      total: resources.length,
      inProgress: resources.filter((r) => ['pending', 'issuing'].includes(r.status)).length,
      failed: resources.filter((r) => r.status === 'failed').length,
    },
  }
}
function counts(requests: AccessRequest[]): RequestCounts {
  const result: RequestCounts = {
    total: requests.length,
    active: 0,
    completed: 0,
    pending: 0,
    issuing: 0,
    granted: 0,
    rejected: 0,
    failed: 0,
  }
  for (const r of requests) {
    result[r.status]++
    if (active(r.status)) result.active++
    else result.completed++
  }
  return result
}
const newIntern: Extract<LoginResult, { status: 'registration_required' }> = {
  status: 'registration_required',
  employeeId: 'intern-ivan',
  firstName: 'Иван',
  secondName: 'Петров',
  email: 'ivan.web@example.test',
  role: 'intern',
  positionId: 'position-frontend',
  team: {
    id: 'team-web',
    name: 'Веб-интерфейсы',
    mentors: [{ id: 'mentor-oleg', firstName: 'Олег', secondName: 'Морозов' }],
  },
  positionOptions: [{ id: 'position-frontend', name: 'Frontend-разработчик' }],
}

export const demoClient: Client = {
  async login(identifier) {
    const value = identifier.trim().toLowerCase()
    const profile = Object.values(state.profiles).find(
      (p) => p.employeeId === value || p.email === value,
    )
    if (profile) return { status: 'ready', profile: copy(profile) }
    if (['intern-ivan', 'ivan.web@example.test'].includes(value)) return copy(newIntern)
    throw new APIError(
      'Не нашли сотрудника. Проверь демо-почту или ID.',
      404,
      'demo_employee_not_found',
    )
  },
  async register(id, input) {
    if (
      id !== newIntern.employeeId ||
      input.mentorId !== 'mentor-oleg' ||
      input.positionId !== 'position-frontend' ||
      !/^\d{4}-\d{2}-\d{2}$/.test(input.internshipStartDate || '')
    )
      throw new APIError('Проверь данные профиля.', 400, 'invalid_input')
    if (state.profiles[id]) return copy(state.profiles[id])
    state.profiles[id] = {
      id: crypto.randomUUID(),
      employeeId: id,
      firstName: newIntern.firstName,
      secondName: newIntern.secondName,
      email: newIntern.email,
      role: 'intern',
      position: newIntern.positionOptions[0].name,
      team: { id: newIntern.team.id, name: newIntern.team.name },
      mentor: newIntern.team.mentors[0],
      internshipStartDate: input.internshipStartDate,
    }
    save()
    return copy(state.profiles[id])
  },
  async accesses(id) {
    return copy(card(id))
  },
  async requests(id, group: RequestGroup = 'all', offset = 0) {
    settle()
    const actor = employee(id)
    const all = state.requests.filter(
      (r) => visible(id, r) && (actor.role !== 'manager' || r.status === 'pending'),
    )
    const items = all.filter(
      (r) =>
        actor.role === 'manager' ||
        group === 'all' ||
        (group === 'active' ? active(r.status) : !active(r.status)),
    )
    return copy({
      items: items.slice(offset, offset + 20),
      total: items.length,
      counts: counts(all),
      limit: 20,
      offset,
    })
  },
  async request(id, requestId) {
    settle()
    return copy(findRequest(id, requestId))
  },
  async confirm(id, draftId) {
    intern(id)
    const previous = state.requests.find((r) => r.draftId === draftId && r.employeeId === id)
    if (previous) return copy(previous)
    const draft = state.drafts[draftId]
    if (!draft || draft.employeeId !== id)
      throw new APIError('Черновик не найден.', 404, 'draft_not_found')
    if (Date.parse(draft.expiresAt) <= Date.now())
      throw new APIError(
        'Черновик устарел. Попроси AI-buddy подготовить новый.',
        409,
        'draft_expired',
      )
    const resource = card(id).resources.find((r) => r.id === draft.resourceId)
    if (!resource?.canRequest)
      throw new APIError(
        'Для этого ресурса уже есть доступ или активная заявка.',
        409,
        'active_request_exists',
      )
    const r: AccessRequest = {
      id: crypto.randomUUID(),
      number: ++state.nextNumber,
      draftId,
      employeeId: id,
      resourceId: draft.resourceId,
      approverId: draft.approverId,
      parameters: copy(draft.parameters),
      status: 'pending',
      createdAt: now(),
      updatedAt: now(),
    }
    state.requests.unshift(r)
    save()
    return copy(r)
  },
  async decide(id, requestId, decision, reason) {
    if (employee(id).role !== 'manager')
      throw new APIError('Только руководитель может принять решение.', 403, 'forbidden')
    const r = findRequest(id, requestId)
    if (r.status !== 'pending') {
      const sameDecision =
        decision === 'reject'
          ? r.status === 'rejected'
          : ['issuing', 'granted', 'failed'].includes(r.status)
      if (sameDecision && r.decisionReason === reason.trim()) return copy(r)
      throw new APIError('По заявке уже принято решение.', 409, 'request_already_decided')
    }
    if (decision === 'reject' && !reason.trim())
      throw new APIError('Укажи причину отказа.', 400, 'invalid_input')
    r.status = decision === 'approve' ? 'issuing' : 'rejected'
    r.decisionReason = reason.trim()
    r.decidedAt = r.updatedAt = now()
    save()
    return copy(r)
  },
  async retry(id, requestId) {
    if (employee(id).role !== 'manager')
      throw new APIError('Повторить выдачу может руководитель.', 403, 'forbidden')
    const r = findRequest(id, requestId)
    if (['issuing', 'granted'].includes(r.status)) return copy(r)
    if (r.status !== 'failed')
      throw new APIError('Повтор выдачи сейчас недоступен.', 409, 'invalid_request_state')
    r.status = 'issuing'
    r.lastError = ''
    r.updatedAt = now()
    save()
    return copy(r)
  },
  async plan(id) {
    const accesses = card(id),
      introDone = !!state.intro[id],
      accessDone = accesses.progress.completed === accesses.progress.total
    const meeting = new Date()
    meeting.setUTCDate(meeting.getUTCDate() + ((3 - meeting.getUTCDay() + 7) % 7 || 7))
    meeting.setUTCHours(8, 30, 0, 0)
    const p = intern(id)
    return {
      tasks: [
        {
          id: 'team-intro',
          title: 'Знакомство с командой',
          subtitle: 'Команда, руководитель и наставник',
          status: introDone ? 'completed' : 'not_started',
        },
        {
          id: 'working-accesses',
          title: 'Рабочие доступы',
          subtitle: 'Ресурсы команды',
          status: accessDone ? 'completed' : 'in_progress',
        },
        {
          id: 'first-task',
          title: 'Первая задача',
          subtitle: 'Учебная задача от руководителя',
          status: introDone && accessDone ? 'coming_soon' : 'locked',
        },
      ],
      progress: { completed: Number(introDone) + Number(accessDone), total: 3 },
      nextTaskId: !introDone ? 'team-intro' : !accessDone ? 'working-accesses' : 'first-task',
      upcomingMeeting: {
        title: 'Встреча с наставником',
        mentorName: `${p.mentor?.firstName} ${p.mentor?.secondName}`,
        startsAt: meeting.toISOString(),
        endsAt: new Date(meeting.getTime() + 30 * 60_000).toISOString(),
        format: 'Онлайн · демо',
        agenda: 'Обсудить первые дни, работу команды и доступы.',
      },
    }
  },
  async intro(id) {
    const p = intern(id)
    return {
      team: copy(p.team),
      description:
        p.team.id === 'team-search'
          ? 'Мы помогаем пользователям Авито быстро находить подходящие объявления. Команда отвечает за поисковый сервис и его качество.'
          : 'Команда создаёт удобные веб-интерфейсы для пользователей Авито.',
      manager: {
        fullName: p.team.id === 'team-search' ? 'Дмитрий Орлов' : 'Анна Соколова',
        position: 'Руководитель команды',
      },
      mentor: {
        fullName: `${p.mentor?.firstName} ${p.mentor?.secondName}`,
        position: 'Твой наставник',
      },
    }
  },
  async completeIntro(id) {
    intern(id)
    state.intro[id] = true
    save()
  },
  async chat(id, taskId) {
    intern(id)
    return copy(state.chats[`${id}:${taskId}`] || { taskId, messages: [] })
  },
  async sendMessage(id, taskId, text, resourceId) {
    const p = intern(id),
      chatKey = `${id}:${taskId}`
    const chat = (state.chats[chatKey] ||= { taskId, messages: [] })
    chat.messages.push({ id: crypto.randomUUID(), role: 'user', text, createdAt: now() })
    const answer = {
      id: crypto.randomUUID(),
      role: 'assistant' as const,
      text: '',
      createdAt: now(),
      sourceLabel: taskId === 'working-accesses' ? 'Каталог ресурсов и заявки' : 'Профиль команды',
      draft: undefined as AccessDraft | undefined,
      request: undefined as AccessRequest | undefined,
    }
    if (taskId === 'team-intro') {
      const intro = await demoClient.intro(id)
      answer.text = /руковод/i.test(text)
        ? `Руководитель команды «${p.team.name}» — ${intro.manager.fullName}. Он согласует доступы и поставит первую учебную задачу.`
        : /команд|занимает/i.test(text)
          ? intro.description
          : `Твой наставник — ${intro.mentor.fullName}. Обсуди с ним работу команды, план первых дней и вопросы по учебной задаче.`
    } else {
      const accesses = card(id)
      const resource = accesses.resources.find(
        (r) =>
          r.id === resourceId ||
          (/запрос|оформ|доступ.*к /i.test(text) &&
            text.toLowerCase().includes(r.service.toLowerCase())),
      )
      if (!resource)
        answer.text = `Для твоей роли нужны ${accesses.resources.map((r) => r.name).join(', ')}. Уже выдано ${accesses.progress.completed} из ${accesses.progress.total}. Выбери один ресурс ниже — подготовлю параметры для подтверждения.`
      else if (resource.status === 'granted')
        answer.text = `Доступ к «${resource.name}» уже есть. Новая заявка не нужна.`
      else if (resource.request) {
        answer.text = `По этому ресурсу уже есть заявка № ${resource.request.number}. Повторно создавать её не нужно.`
        answer.request = findRequest(id, resource.request.id)
      } else {
        const draft: AccessDraft & { employeeId: string } = {
          id: crypto.randomUUID(),
          employeeId: id,
          resourceId: resource.id,
          approverId: p.team.id === 'team-search' ? 'manager-dmitry' : 'manager-anna',
          parameters: {
            resourceName: resource.name,
            resourceService: resource.service,
            resourceExternalId: resource.id,
            accessLevel: resource.accessLevel,
            purpose: resource.purpose,
            approverName: p.team.id === 'team-search' ? 'Дмитрий Орлов' : 'Анна Соколова',
          },
          createdAt: now(),
          expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
        }
        state.drafts[draft.id] = draft
        answer.draft = copy(draft)
        answer.text =
          'Подготовил черновик одного доступа. Проверь параметры. После твоего подтверждения заявка поступит руководителю.'
      }
    }
    chat.messages.push(answer)
    save()
  },
}
