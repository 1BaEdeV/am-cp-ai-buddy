package onboarding

import "context"

func (s *Service) GetPlan(ctx context.Context, employeeID string) (OnboardingPlan, error) {
	intro, err := s.GetTeamIntro(ctx, employeeID)
	if err != nil {
		return OnboardingPlan{}, err
	}

	introCompleted := intro.Status == "completed"

	accessCompleted := false
	accessProgress := Progress{
		Completed: 0,
		Total:     4,
	}
	accessStatus := "not_started"

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

	firstTaskStatus := "locked"
	if introCompleted && accessCompleted {
		firstTaskStatus = "coming_soon"
		tasks[2].Status = firstTaskStatus
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
		AccessProgress: accessProgress,
		NextTaskID:     nextTaskID,
		AccessRequestSummary: AccessRequestSummary{
			Pending:    0,
			InProgress: 0,
			Processed:  0,
		},
		UpcomingMeeting: upcomingMeeting,
	}, nil
}
func (s *Service) GetFirstTaskDetails(
	ctx context.Context,
	employeeID string,
) (TaskDetails, error) {
	plan, err := s.GetPlan(ctx, employeeID)
	if err != nil {
		return TaskDetails{}, err
	}

	status := "locked"
	for _, task := range plan.Tasks {
		if task.ID == "first-task" {
			status = task.Status
			break
		}
	}

	return TaskDetails{
		ID:          "first-task",
		Title:       "Первая задача",
		Subtitle:    "Учебная задача от руководителя",
		Status:      status,
		Description: "Познакомься с командой и получи обязательные доступы.",
		ChatEnabled: false,
	}, nil
}