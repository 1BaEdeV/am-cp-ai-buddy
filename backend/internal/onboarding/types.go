package onboarding

type Contact struct { // имя, роль и необязательный email руководителя или наставника.
	FullName string `json:"fullName"`
	Position string `json:"position"`
	Email    string `json:"email,omitempty"`
}

type TeamRef struct { /// идентификатор и название команды в карточке знакомства.
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TeamIntro struct { //  данные о команде, руководителе и наставнике для карточки знакомства.
	Team        TeamRef `json:"team"`
	Description string  `json:"description"`
	Manager     Contact `json:"manager"`
	Mentor      Contact `json:"mentor"`
}

type TaskDetails struct { // полная карточка одного этапа адаптации, открытая стажёром.
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Subtitle        string           `json:"subtitle"`
	Status          string           `json:"status"`
	Description     string           `json:"description"`
	ChatEnabled     bool             `json:"chatEnabled"`
	TeamIntro       *TeamIntro       `json:"teamIntro,omitempty"`
	WorkingAccesses *WorkingAccesses `json:"workingAccesses,omitempty"`
}

type TaskSummary struct { // короткое описание этапа для списка «Мой план».
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Status   string `json:"status"`
}

type PlanProgress struct { //число завершённых этапов из общего числа этапов плана.
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type Progress struct { // число полученных доступов из числа обязательных ресурсов.
	Completed  int `json:"completed"`
	Total      int `json:"total"`
	InProgress int `json:"-"`
	Failed     int `json:"-"`
}

type WorkingAccesses struct { // содержит обязательные ресурсы команды и прогресс их выдачи.
	Resources []AccessResource `json:"resources"`
	Progress  Progress         `json:"progress"`
}

type AccessResource struct { // описывает один ресурс, который нужен стажёру для работы.
	ID          string `json:"id"`
	Service     string `json:"service"`
	Name        string `json:"name"`
	AccessLevel string `json:"accessLevel"`
	Purpose     string `json:"purpose"`
	Status      string `json:"status"`
	CanRequest  bool   `json:"canRequest"`
	RequestID   string `json:"requestId,omitempty"`
}

type AccessRequestSummary struct { // количество заявок по группам статусов для сайдбара.
	Pending    int `json:"pending"`
	InProgress int `json:"inProgress"`
	Processed  int `json:"processed"`
}

type Meeting struct { // данные о ближайшей встрече с наставником.
	Title      string `json:"title"`
	MentorName string `json:"mentorName"`
	StartsAt   string `json:"startsAt"`
	EndsAt     string `json:"endsAt"`
	Format     string `json:"format"`
	Agenda     string `json:"agenda"`
}

type OnboardingPlan struct { // все три этапа адаптации, общий прогресс и данные сайдбара.
	Tasks                []TaskSummary        `json:"tasks"`
	Progress             PlanProgress         `json:"progress"`
	AccessProgress       Progress             `json:"accessProgress"`
	NextTaskID           string               `json:"nextTaskId"`
	AccessRequestSummary AccessRequestSummary `json:"accessRequestSummary"`
	UpcomingMeeting      *Meeting             `json:"upcomingMeeting,omitempty"`
}
