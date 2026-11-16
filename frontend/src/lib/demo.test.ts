import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { demoClient as client, resetDemo } from './demo'

const intern = 'intern-alina',
  manager = 'manager-dmitry'
async function prepare(resource = 'gitlab-search-api') {
  await client.sendMessage(intern, 'working-accesses', 'Запроси доступ', resource)
  const chat = await client.chat(intern, 'working-accesses')
  const draft = chat.messages.at(-1)?.draft
  expect(draft).toBeDefined()
  return draft!
}
beforeEach(resetDemo)
afterEach(() => vi.useRealTimers())
describe('Демо-сценарий доступов', () => {
  it.each([
    [' Сидоров  Алексей Сергеевич ', 'Алексей', 'Сидоров', 'Сергеевич'],
    ['Иванова-Петрова Анна-Мария', 'Анна-Мария', 'Иванова-Петрова', undefined],
  ])(
    'сохраняет введённое ФИО %s вместо имени демо-сотрудника',
    async (fullName, firstName, secondName, middleName) => {
      const input = {
        fullName,
        positionId: 'position-frontend',
        mentorId: 'mentor-oleg',
        internshipStartDate: '2026-11-16',
      }
      const saved = await client.register('intern-ivan', input)
      expect(saved).toMatchObject({ firstName, secondName, middleName })
      expect(await client.login('intern-ivan')).toEqual({ status: 'ready', profile: saved })
      expect((await client.intro('intern-ivan')).trainee?.fullName).toBe(
        [firstName, middleName, secondName].filter(Boolean).join(' '),
      )
      expect(await client.register('intern-ivan', input)).toEqual(saved)
      await expect(
        client.register('intern-ivan', { ...input, fullName: 'Петров Иван' }),
      ).rejects.toMatchObject({ code: 'profile_already_exists' })
    },
  )
  it.each([
    '',
    '  ',
    'Иван',
    'Иванов Иван123',
    '--- Иван',
    'Иванов Иван Иванович Ещё',
    'Я'.repeat(150) + ' Иван',
  ])('не сохраняет некорректное ФИО %s', async (fullName) => {
    await expect(
      client.register('intern-ivan', {
        fullName,
        positionId: 'position-frontend',
        mentorId: 'mentor-oleg',
        internshipStartDate: '2026-11-16',
      }),
    ).rejects.toMatchObject({ code: 'invalid_input' })
    expect((await client.login('intern-ivan')).status).toBe('registration_required')
  })
  it.each([
    ['team-web', 'position-frontend', 'Frontend-разработчик', 'gitlab-onboarding', 'Developer', 4],
    [
      'team-web',
      'position-backend-go',
      'Backend-разработчик Go',
      'gitlab-onboarding',
      'Developer',
      3,
    ],
    ['team-web', 'position-qa', 'Инженер по тестированию', 'gitlab-onboarding', 'Reporter', 4],
    ['team-web', 'position-designer', 'Продуктовый дизайнер', 'figma-design', 'Editor', 3],
    [
      'team-search',
      'position-backend-go',
      'Backend-разработчик Go',
      'gitlab-search-api',
      'Developer',
      4,
    ],
    ['team-search', 'position-qa', 'Инженер по тестированию', 'gitlab-onboarding', 'Reporter', 4],
    ['team-search', 'position-data-analyst', 'Аналитик данных', 'postgres-training', 'read', 5],
  ] as const)(
    'связывает должность %s/%s с доступами и параметрами заявки',
    async (teamId, positionId, positionName, resourceId, level, total) => {
      const form = await client.login('intern-ivan')
      if (form.status !== 'registration_required') throw new Error('Ожидалась регистрация')
      const team = form.teamOptions!.find((option) => option.id === teamId)!
      expect(team.positionOptions!.length).toBeGreaterThanOrEqual(3)
      expect(team.positionOptions).toContainEqual({ id: positionId, name: positionName })
      const profile = await client.register('intern-ivan', {
        teamId,
        positionId,
        mentorId: team.mentors[0].id,
        internshipStartDate: '2026-11-16',
      })
      expect(profile.position).toBe(positionName)
      const card = await client.accesses('intern-ivan')
      expect(card.progress.total).toBe(total)
      expect(card.resources.find((resource) => resource.id === resourceId)?.accessLevel).toBe(level)
      await client.sendMessage('intern-ivan', 'working-accesses', 'Запроси доступ', resourceId)
      const draft = (await client.chat('intern-ivan', 'working-accesses')).messages.at(-1)!.draft!
      const request = await client.confirm('intern-ivan', draft.id)
      expect(request.parameters.accessLevel).toBe(level)
      expect(request.approverId).toBe(teamId === 'team-web' ? 'manager-anna' : 'manager-dmitry')
    },
  )
  it('связывает выбранную команду с ресурсами, наставником и согласующим', async () => {
    const input = {
      teamId: 'team-search',
      positionId: 'position-backend-go',
      mentorId: 'mentor-sasha',
      internshipStartDate: '2026-11-16',
    }
    const result = await client.register('intern-ivan', input)
    expect(result.team.id).toBe('team-search')
    expect(result.mentor?.id).toBe('mentor-sasha')
    expect((await client.intro('intern-ivan')).mentor.fullName).toBe('Саша Лебедев')
    expect((await client.accesses('intern-ivan')).resources.map((r) => r.id)).toContain(
      'gitlab-search-api',
    )
    expect(await client.register('intern-ivan', input)).toEqual(result)
    await expect(
      client.register('intern-ivan', { ...input, teamId: 'team-web', mentorId: 'mentor-oleg' }),
    ).rejects.toMatchObject({ code: 'profile_already_exists' })
    await expect(
      client.register('intern-ivan', { ...input, mentorId: 'mentor-katya' }),
    ).rejects.toMatchObject({ code: 'profile_already_exists' })
    await client.sendMessage(
      'intern-ivan',
      'working-accesses',
      'Запроси доступ',
      'gitlab-search-api',
    )
    expect(
      (await client.chat('intern-ivan', 'working-accesses')).messages.at(-1)?.draft?.approverId,
    ).toBe('manager-dmitry')
  })
  it('проходит от 2/4 до 4/4 с отдельным подтверждением каждого ресурса', async () => {
    vi.useFakeTimers()
    expect((await client.accesses(intern)).progress.completed).toBe(2)
    for (const [index, resource] of ['gitlab-search-api', 'grafana-search'].entries()) {
      const draft = await prepare(resource)
      expect((await client.requests(intern)).total).toBe(index)
      const request = await client.confirm(intern, draft.id)
      expect((await client.confirm(intern, draft.id)).id).toBe(request.id)
      expect(request.parameters).toEqual(draft.parameters)
      await client.decide(manager, request.id, 'approve', '')
      expect((await client.accesses(intern)).progress.completed).toBe(index + 2)
      expect((await client.requests(manager)).total).toBe(0)
      await vi.advanceTimersByTimeAsync(1000)
      expect((await client.request(intern, request.id)).status).toBe('granted')
      expect((await client.accesses(intern)).progress.completed).toBe(index + 3)
    }
    await client.completeIntro(intern)
    const plan = await client.plan(intern)
    expect(plan.progress.completed).toBe(2)
    expect(plan.tasks[2].status).toBe('coming_soon')
    expect((await client.requests(intern, 'completed')).total).toBe(2)
  })
  it('сохраняет отказ, разрешает новый черновик и защищает чужую заявку', async () => {
    const request = await client.confirm(intern, (await prepare()).id)
    await expect(client.decide(manager, request.id, 'reject', ' ')).rejects.toMatchObject({
      status: 400,
    })
    await client.decide(manager, request.id, 'reject', 'Уточни назначение')
    expect((await client.request(intern, request.id)).decisionReason).toBe('Уточни назначение')
    await expect(client.request('manager-anna', request.id)).rejects.toMatchObject({ status: 404 })
    await expect(client.decide(intern, request.id, 'approve', '')).rejects.toMatchObject({
      status: 403,
    })
    const replacement = await client.confirm(intern, (await prepare()).id)
    expect(replacement.id).not.toBe(request.id)
    expect((await client.requests(intern)).counts).toMatchObject({
      active: 1,
      completed: 1,
      rejected: 1,
    })
  })
  it('не подтверждает истёкший черновик и не готовит дубль активной заявки', async () => {
    vi.useFakeTimers()
    const draft = await prepare()
    await vi.advanceTimersByTimeAsync(16 * 60_000)
    await expect(client.confirm(intern, draft.id)).rejects.toMatchObject({ code: 'draft_expired' })
    const next = await prepare()
    const request = await client.confirm(intern, next.id)
    await client.sendMessage(intern, 'working-accesses', 'Запроси доступ', 'gitlab-search-api')
    const message = (await client.chat(intern, 'working-accesses')).messages.at(-1)!
    expect(message.draft).toBeUndefined()
    expect(message.request?.id).toBe(request.id)
    expect((await client.requests(intern)).total).toBe(1)
  })
  it('разделяет переписку по заданиям и пользователей по командам', async () => {
    await client.sendMessage(intern, 'team-intro', 'Кто мой наставник?')
    expect((await client.chat(intern, 'working-accesses')).messages).toHaveLength(0)
    expect((await client.chat(intern, 'team-intro')).messages.at(-1)?.text).toContain(
      'Катя Волкова',
    )
    await client.register('intern-ivan', {
      positionId: 'position-frontend',
      mentorId: 'mentor-oleg',
      internshipStartDate: '2026-11-15',
    })
    expect((await client.accesses('intern-ivan')).resources.map((r) => r.id)).toContain(
      'figma-design',
    )
    expect((await client.accesses('intern-ivan')).resources.map((r) => r.id)).not.toContain(
      'gitlab-search-api',
    )
    await expect(client.confirm('intern-ivan', (await prepare()).id)).rejects.toMatchObject({
      status: 404,
    })
  })
})
