import { vi } from 'vitest'
import type {
  AccessDraft,
  AccessRequest,
  Client,
  LoginResult,
  OnboardingPlan,
  Profile,
  RequestList,
  TeamIntro,
  WorkingAccesses,
} from '../lib/types'

export const profile: Profile = {
  id: 'profile-alina',
  employeeId: 'intern-alina',
  firstName: 'Алина',
  secondName: 'Смирнова',
  email: 'alina@example.test',
  role: 'intern',
  position: 'Разработчик',
  team: { id: 'search', name: 'Поиск' },
  mentor: { id: 'katya', firstName: 'Катя', secondName: 'Волкова' },
  internshipStartDate: '2026-11-10',
}
export const manager: Profile = {
  ...profile,
  employeeId: 'manager-dmitry',
  firstName: 'Дмитрий',
  secondName: 'Орлов',
  role: 'manager',
  mentor: undefined,
  internshipStartDate: undefined,
}
export const parameters = {
  resourceName: 'GitLab · search-api',
  resourceService: 'gitlab',
  resourceExternalId: 'search-api',
  accessLevel: 'Developer',
  purpose: 'Работа с кодом',
  approverName: 'Дмитрий Орлов',
}
export const draft: AccessDraft = {
  id: 'draft-1',
  resourceId: 'gitlab',
  approverId: manager.employeeId,
  parameters,
  createdAt: '2026-11-10T09:00:00Z',
  expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
}
export const request: AccessRequest = {
  id: 'request-1',
  number: 1041,
  draftId: draft.id,
  employeeId: profile.employeeId,
  resourceId: draft.resourceId,
  approverId: manager.employeeId,
  parameters,
  status: 'pending',
  createdAt: draft.createdAt,
  updatedAt: draft.createdAt,
}
export const accesses: WorkingAccesses = {
  resources: [
    {
      id: 'wiki',
      service: 'wiki',
      name: 'Вики',
      accessLevel: 'read',
      purpose: 'Инструкции',
      status: 'granted',
      canRequest: false,
    },
    {
      id: 'gitlab',
      service: 'gitlab',
      name: parameters.resourceName,
      accessLevel: 'Developer',
      purpose: parameters.purpose,
      status: 'requestable',
      canRequest: true,
    },
    {
      id: 'grafana',
      service: 'grafana',
      name: 'Grafana',
      accessLevel: 'Viewer',
      purpose: 'Метрики',
      status: 'pending',
      canRequest: false,
      request: { id: 'request-2', number: 1042, status: 'pending' },
    },
  ],
  progress: { completed: 1, total: 3, inProgress: 1, failed: 0 },
}
export const plan: OnboardingPlan = {
  tasks: [
    { id: 'team-intro', title: 'Знакомство с командой', subtitle: 'Люди', status: 'not_started' },
    {
      id: 'working-accesses',
      title: 'Рабочие доступы',
      subtitle: 'Ресурсы',
      status: 'in_progress',
    },
    { id: 'first-task', title: 'Первая задача', subtitle: 'Учебная задача', status: 'locked' },
  ],
  progress: { completed: 0, total: 3 },
  nextTaskId: 'team-intro',
  upcomingMeeting: {
    title: 'Встреча с наставником',
    mentorName: 'Катя Волкова',
    startsAt: '2026-11-18T08:30:00Z',
    endsAt: '2026-11-18T09:00:00Z',
    format: 'Онлайн',
    agenda: 'Обсудить первые дни',
  },
}
export const intro: TeamIntro = {
  team: profile.team,
  description: 'Развиваем поиск объявлений',
  manager: { fullName: 'Дмитрий Орлов', position: 'Руководитель' },
  mentor: { fullName: 'Катя Волкова', position: 'Наставник' },
}
export const list: RequestList = {
  items: [request],
  total: 1,
  limit: 20,
  offset: 0,
  counts: {
    total: 1,
    active: 1,
    completed: 0,
    pending: 1,
    issuing: 0,
    granted: 0,
    rejected: 0,
    failed: 0,
  },
}
export const registration: Extract<LoginResult, { status: 'registration_required' }> = {
  status: 'registration_required',
  employeeId: 'intern-ivan',
  firstName: 'Иван',
  secondName: 'Петров',
  email: 'ivan@example.test',
  role: 'intern',
  positionId: 'frontend',
  team: {
    id: 'web',
    name: 'Веб',
    mentors: [{ id: 'oleg', firstName: 'Олег', secondName: 'Морозов' }],
  },
  positionOptions: [
    { id: 'frontend', name: 'Frontend' },
    { id: 'backend', name: 'Backend' },
  ],
}
export const mockClient = {
  login: vi.fn<Client['login']>(),
  register: vi.fn<Client['register']>(),
  accesses: vi.fn<Client['accesses']>(),
  requests: vi.fn<Client['requests']>(),
  request: vi.fn<Client['request']>(),
  confirm: vi.fn<Client['confirm']>(),
  decide: vi.fn<Client['decide']>(),
  retry: vi.fn<Client['retry']>(),
  plan: vi.fn<Client['plan']>(),
  intro: vi.fn<Client['intro']>(),
  completeIntro: vi.fn<Client['completeIntro']>(),
  chat: vi.fn<Client['chat']>(),
  sendMessage: vi.fn<Client['sendMessage']>(),
}
export function resetClient() {
  for (const mock of Object.values(mockClient)) mock.mockReset()
  mockClient.login.mockResolvedValue({ status: 'ready', profile })
  mockClient.register.mockResolvedValue(profile)
  mockClient.accesses.mockResolvedValue(accesses)
  mockClient.requests.mockResolvedValue(list)
  mockClient.request.mockResolvedValue(request)
  mockClient.confirm.mockResolvedValue(request)
  mockClient.decide.mockResolvedValue(request)
  mockClient.retry.mockResolvedValue(request)
  mockClient.plan.mockResolvedValue(plan)
  mockClient.intro.mockResolvedValue(intro)
  mockClient.completeIntro.mockResolvedValue(undefined)
  mockClient.chat.mockResolvedValue({ taskId: 'team-intro', messages: [] })
  mockClient.sendMessage.mockResolvedValue(undefined)
}
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((done, fail) => {
    resolve = done
    reject = fail
  })
  return { promise, resolve, reject }
}
