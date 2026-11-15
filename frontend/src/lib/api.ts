import type {
  AccessDraft,
  AccessRequest,
  ChatMessage,
  Client,
  LoginResult,
  OnboardingPlan,
  Profile,
  RegistrationInput,
  RequestGroup,
  RequestList,
  TaskChat,
  TaskId,
  TeamIntro,
  WorkingAccesses,
} from './types'

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public code: string,
  ) {
    super(message)
    this.name = 'APIError'
  }
}
type ChatResponse = Omit<TaskChat, 'messages' | 'latestRun'> & {
  messages: (ChatMessage & { accessDraft?: AccessDraft })[]
  latestRun?: Omit<NonNullable<TaskChat['latestRun']>, 'error'> & { error?: { message: string } }
}
const base = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
async function request<T>(
  path: string,
  employeeId?: string,
  body?: unknown,
  extraHeaders?: Record<string, string>,
): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${base}${path}`, {
      method: body === undefined ? 'GET' : 'POST',
      headers: {
        ...(employeeId ? { 'X-Demo-Employee-Id': employeeId } : {}),
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        ...extraHeaders,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(15_000),
    })
  } catch {
    throw new APIError(
      'Не удалось связаться с сервером. Проверь подключение и повтори попытку.',
      0,
      'network_error',
    )
  }
  if (response.status === 204) return undefined as T
  const data = await response.json().catch(() => null)
  if (!response.ok) {
    throw new APIError(
      data?.message ||
        (response.status === 404
          ? 'Этот раздел пока недоступен. Попробуй вернуться к нему позже.'
          : 'Не удалось выполнить действие. Повтори попытку.'),
      response.status,
      data?.code || 'http_error',
    )
  }
  if (data === null)
    throw new APIError('Сервер вернул неполный ответ. Повтори попытку.', 502, 'invalid_response')
  return data as T
}
export const apiClient: Client = {
  login: (identifier: string) => request<LoginResult>('/auth/login', undefined, { identifier }),
  register: (id: string, input: RegistrationInput) => request<Profile>('/auth/register', id, input),
  accesses: async (id: string) =>
    (await request<{ workingAccesses: WorkingAccesses }>('/onboarding/tasks/working-accesses', id))
      .workingAccesses,
  requests: (id: string, group: RequestGroup = 'all', offset = 0) =>
    request<RequestList>(
      `/access-requests?${new URLSearchParams({ group, limit: '20', offset: String(offset) })}`,
      id,
    ),
  request: (id: string, requestId: string) =>
    request<AccessRequest>(`/access-requests/${encodeURIComponent(requestId)}`, id),
  confirm: (id: string, draftId: string) =>
    request<AccessRequest>(`/access-requests/drafts/${encodeURIComponent(draftId)}/confirm`, id, {
      confirmed: true,
    }),
  decide: (id: string, requestId: string, decision: 'approve' | 'reject', reason: string) =>
    request<AccessRequest>(`/access-requests/${encodeURIComponent(requestId)}/decision`, id, {
      decision,
      reason,
    }),
  retry: (id: string, requestId: string) =>
    request<AccessRequest>(`/access-requests/${encodeURIComponent(requestId)}/retry`, id, {}),
  plan: (id: string) => request<OnboardingPlan>('/onboarding', id),
  intro: async (id: string) =>
    (await request<{ teamIntro: TeamIntro }>('/onboarding/tasks/team-intro', id)).teamIntro,
  completeIntro: async (id: string) => {
    await request('/onboarding/tasks/team-intro/complete', id, {})
  },
  chat: async (id: string, task: TaskId) => {
    const chat = await request<ChatResponse>(`/onboarding/tasks/${task}/chat`, id)
    return {
      ...chat,
      messages: chat.messages.map(({ accessDraft, ...message }) => ({
        ...message,
        draft: accessDraft,
      })),
      latestRun: chat.latestRun && { ...chat.latestRun, error: chat.latestRun.error?.message },
    }
  },
  sendMessage: async (id: string, task: TaskId, text: string) => {
    await request(
      `/onboarding/tasks/${task}/chat/messages`,
      id,
      { text },
      { 'Idempotency-Key': crypto.randomUUID() },
    )
  },
}
