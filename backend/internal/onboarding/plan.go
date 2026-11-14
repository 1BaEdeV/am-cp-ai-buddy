package onboarding

import "context"

func (s *Service) GetPlan(ctx context.Context, employeeID string) (OnboardingPlan, error) {
	intro, err := s.GetTeamIntro(ctx, employeeID)
	if err != nil {
		return OnboardingPlan{}, err
	}

	accesses := demoWorkingAccesses()
	introCompleted := intro.Status == "completed"
	accessCompleted := accesses.Progress.Total > 0 &&
		accesses.Progress.Completed == accesses.Progress.Total

	accessStatus := "not_started"
	if accesses.Progress.Completed > 0 {
		accessStatus = "in_progress"
	}
	if accessCompleted {
		accessStatus = "completed"
	}

	tasks := []TaskSummary{
		{
			ID:       "team-intro",
			Title:    intro.Title,
			Subtitle: intro.Subtitle,
			Status:   intro.Status,
		},
		{
			ID:       "working-accesses",
			Title:    "Рабочие доступы",
			Subtitle: "Ресурсы команды",
			Status:   accessStatus,
		},
		{
			ID:       "first-task",
			Title:    "Первая задача",
			Subtitle: "Учебная задача от руководителя",
			Status:   "locked",
		},
	}

	completedTasks := 0
	if introCompleted {
		completedTasks++
	}
	if accessCompleted {
		completedTasks++
	}

	if introCompleted && accessCompleted {
		tasks[2].Status = "coming_soon"
	}

	nextTaskID := "team-intro"
	if introCompleted {
		nextTaskID = "working-accesses"
	}
	if introCompleted && accessCompleted {
		nextTaskID = "first-task"
	}

	var upcomingMeeting *Meeting
	if intro.TeamIntro != nil {
		upcomingMeeting = &Meeting{
			Title:      "Встреча с наставником",
			MentorName: intro.TeamIntro.Mentor.FullName,
			StartsAt:   "2026-11-16T11:30:00+03:00",
			EndsAt:     "2026-11-16T12:00:00+03:00",
			Format:     "Онлайн",
			Agenda:     "Обсудить первые дни в команде и рабочие доступы.",
		}
	}

	return OnboardingPlan{
		Tasks:          tasks,
		Progress:       PlanProgress{Completed: completedTasks, Total: 3},
		AccessProgress: accesses.Progress,
		NextTaskID:     nextTaskID,
		AccessRequestSummary: AccessRequestSummary{
			Pending:    0,
			InProgress: 0,
			Processed:  0,
		},
		UpcomingMeeting: upcomingMeeting,
	}, nil
}

// GetTaskDetails собирает подробности выбранного этапа.
func (s *Service) GetTaskDetails(
	ctx context.Context,
	employeeID string,
	taskID string,
) (TaskDetails, error) {
	switch taskID {
	case "team-intro":
		return s.GetTeamIntro(ctx, employeeID)

	case "working-accesses":
		plan, err := s.GetPlan(ctx, employeeID)
		if err != nil {
			return TaskDetails{}, err
		}

		return TaskDetails{
			ID:          "working-accesses",
			Title:       "Рабочие доступы",
			Subtitle:    "Ресурсы команды",
			Status:      taskStatus(plan, "working-accesses"),
			Description: "Проверь доступы к обязательным ресурсам команды.",
			ChatEnabled: true,
			WorkingAccesses: func() *WorkingAccesses {
				accesses := demoWorkingAccesses()
				return &accesses
			}(),
		}, nil

	case "first-task":
		plan, err := s.GetPlan(ctx, employeeID)
		if err != nil {
			return TaskDetails{}, err
		}

		return TaskDetails{
			ID:          "first-task",
			Title:       "Первая задача",
			Subtitle:    "Учебная задача от руководителя",
			Status:      taskStatus(plan, "first-task"),
			Description: "Познакомься с командой и получи обязательные доступы.",
			ChatEnabled: false,
		}, nil

	default:
		return TaskDetails{}, ErrInvalidTask
	}
}

func taskStatus(plan OnboardingPlan, taskID string) string {
	for _, task := range plan.Tasks {
		if task.ID == taskID {
			return task.Status
		}
	}
	return "locked"
}

// Временные данные для демо; позже их заменит сервис доступов.
func demoWorkingAccesses() WorkingAccesses {
	return WorkingAccesses{
		Resources: []AccessResource{
			{
				ID:          "team-wiki",
				Service:     "wiki",
				Name:        "Вики команды",
				AccessLevel: "Чтение",
				Purpose:     "Инструкции и процессы команды",
				Status:      "granted",
				CanRequest:  false,
			},
			{
				ID:          "youtrack-search",
				Service:     "youtrack",
				Name:        "YouTrack",
				AccessLevel: "Участник проекта",
				Purpose:     "Работа с задачами команды",
				Status:      "granted",
				CanRequest:  false,
			},
			{
				ID:          "gitlab-search-api",
				Service:     "gitlab",
				Name:        "GitLab · search-api",
				AccessLevel: "Developer",
				Purpose:     "Код и работа с ветками",
				Status:      "requestable",
				CanRequest:  true,
			},
			{
				ID:          "grafana-search",
				Service:     "grafana",
				Name:        "Grafana · Поиск",
				AccessLevel: "Чтение",
				Purpose:     "Дашборды тестового окружения",
				Status:      "requestable",
				CanRequest:  true,
			},
		},
		Progress: Progress{
			Completed: 2,
			Total:     4,
		},
	}
}