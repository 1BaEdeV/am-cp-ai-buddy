package onboarding

type Contact struct {
	FullName string `json:"fullName"`
	Position string `json:"position"`
	Email    string `json:"email,omitempty"`
}

type TeamRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TeamIntro struct {
	Team        TeamRef `json:"team"`
	Description string  `json:"description"`
	Manager     Contact `json:"manager"`
	Mentor      Contact `json:"mentor"`
}

type TaskDetails struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Subtitle    string     `json:"subtitle"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	ChatEnabled bool       `json:"chatEnabled"`
	TeamIntro   *TeamIntro `json:"teamIntro,omitempty"`
}