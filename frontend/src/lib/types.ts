export type Role = 'intern' | 'manager'
export type TaskId = 'team-intro' | 'working-accesses' | 'first-task'
export type RequestStatus = 'pending' | 'issuing' | 'granted' | 'rejected' | 'failed'
export type RequestGroup = 'all' | 'active' | 'completed'
export interface Option {
  id: string
  name: string
}
export interface Mentor {
  id: string
  firstName: string
  secondName: string
}
export interface RegistrationTeam extends Option {
  mentors: Mentor[]
  positionOptions?: Option[]
}
export interface Profile {
  id: string
  employeeId: string
  firstName: string
  secondName: string
  middleName?: string
  email: string
  role: Role
  position: string
  team: Option
  mentor?: Mentor
  internshipStartDate?: string
}
export type LoginResult =
  | { status: 'ready'; profile: Profile }
  | {
      status: 'registration_required'
      employeeId: string
      firstName: string
      secondName: string
      middleName?: string
      email: string
      role: Role
      positionId: string
      team: RegistrationTeam
      teamOptions?: RegistrationTeam[]
      positionOptions: Option[]
    }
export interface RegistrationInput {
  fullName?: string
  teamId?: string
  positionId: string
  mentorId?: string
  internshipStartDate?: string
}
export interface Parameters {
  resourceName: string
  resourceService: string
  resourceExternalId: string
  accessLevel: string
  purpose: string
  approverName: string
}
export interface AccessDraft {
  id: string
  resourceId: string
  approverId: string
  parameters: Parameters
  createdAt: string
  expiresAt: string
}
export interface AccessRequest {
  id: string
  number: number
  draftId: string
  employeeId: string
  resourceId: string
  approverId: string
  parameters: Parameters
  status: RequestStatus
  createdAt: string
  updatedAt: string
  decidedAt?: string
  decisionReason?: string
  grantedAt?: string
  lastError?: string
}
export interface RequiredResource {
  id: string
  service: string
  name: string
  accessLevel: string
  purpose: string
  status: 'requestable' | 'granted' | 'pending' | 'issuing' | 'failed'
  canRequest: boolean
  request?: Pick<AccessRequest, 'id' | 'number' | 'status'>
}
export interface WorkingAccesses {
  resources: RequiredResource[]
  progress: { completed: number; total: number; inProgress: number; failed: number }
}
export interface RequestCounts {
  total: number
  active: number
  completed: number
  pending: number
  issuing: number
  granted: number
  rejected: number
  failed: number
}
export interface RequestList {
  items: AccessRequest[]
  total: number
  counts: RequestCounts
  limit: number
  offset: number
}
export interface TaskSummary {
  id: TaskId
  title: string
  subtitle: string
  status: 'not_started' | 'in_progress' | 'completed' | 'locked' | 'coming_soon'
}
export interface OnboardingPlan {
  tasks: TaskSummary[]
  progress: { completed: number; total: number }
  nextTaskId: TaskId
  upcomingMeeting?: {
    title: string
    mentorName: string
    startsAt: string
    endsAt: string
    format: string
    agenda: string
  }
}
export interface TeamIntro {
  team: Option
  description: string
  manager: { fullName: string; position: string; email?: string; about?: string }
  mentor: { fullName: string; position: string; email?: string; about?: string }
  trainee?: { fullName: string; position: string; internshipStartDate?: string }
}
export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  text: string
  createdAt: string
  sourceLabel?: string
  draft?: AccessDraft
  request?: AccessRequest
}
export interface TaskChat {
  taskId: TaskId
  messages: ChatMessage[]
  latestRun?: {
    id: string
    userMessageId: string
    status: 'queued' | 'running' | 'completed' | 'failed'
    error?: string
  }
}
export interface Client {
  login(identifier: string): Promise<LoginResult>
  register(employeeId: string, input: RegistrationInput): Promise<Profile>
  accesses(employeeId: string): Promise<WorkingAccesses>
  requests(employeeId: string, group?: RequestGroup, offset?: number): Promise<RequestList>
  request(employeeId: string, id: string): Promise<AccessRequest>
  confirm(employeeId: string, draftId: string): Promise<AccessRequest>
  decide(
    employeeId: string,
    id: string,
    decision: 'approve' | 'reject',
    reason: string,
  ): Promise<AccessRequest>
  retry(employeeId: string, id: string): Promise<AccessRequest>
  plan(employeeId: string): Promise<OnboardingPlan>
  intro(employeeId: string): Promise<TeamIntro>
  completeIntro(employeeId: string): Promise<void>
  chat(employeeId: string, taskId: TaskId): Promise<TaskChat>
  sendMessage(employeeId: string, taskId: TaskId, text: string, resourceId?: string): Promise<void>
}
