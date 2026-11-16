package auth

import (
	"errors"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

var (
	ErrInvalidInput            = errors.New("проверь данные формы")
	ErrEmployeeNotFound        = errors.New("сотрудник не найден")
	ErrForbidden               = errors.New("роль сотрудника не поддерживается")
	ErrRegistrationUnavailable = errors.New("справочники для регистрации не заполнены")
	ErrProfileAlreadyExists    = errors.New("профиль уже сохранён с другими данными")
	ErrProfileRequired         = errors.New("заполни профиль сотрудника")
)

// LoginStatus показывает, можно ли открыть приложение или нужно заполнить профиль.
type LoginStatus string

const (
	LoginReady             LoginStatus = "ready"
	LoginNeedsRegistration LoginStatus = "registration_required"
)

// LoginResult возвращает готовый профиль либо данные для формы первого входа.
// Вариант ответа определяется полем Status.
type LoginResult struct {
	Status          LoginStatus         `json:"status"`
	Profile         *Profile            `json:"profile,omitempty"`
	EmployeeID      string              `json:"employeeId,omitempty"`
	FirstName       string              `json:"firstName,omitempty"`
	SecondName      string              `json:"secondName,omitempty"`
	Email           string              `json:"email,omitempty"`
	Role            models.EmployeeRole `json:"role,omitempty"`
	PositionID      string              `json:"positionId,omitempty"`
	Team            *TeamOption         `json:"team,omitempty"`
	TeamOptions     []TeamOption        `json:"teamOptions,omitempty"`
	PositionOptions []Option            `json:"positionOptions,omitempty"`
}

// RegistrationInput содержит выбор пользователя при первом заполнении профиля.
// Стажёр также выбирает команду, её наставника и дату начала стажировки.
type RegistrationInput struct {
	TeamID              string `json:"teamId,omitempty"`
	PositionID          string `json:"positionId,omitempty"`
	MentorID            string `json:"mentorId,omitempty"`
	InternshipStartDate string `json:"internshipStartDate,omitempty"` // Календарная дата в формате YYYY-MM-DD.
}

// Profile содержит данные зарегистрированного сотрудника для ответа API.
// Роль определяет, какой экран откроет интерфейс.
type Profile struct {
	ID                  string              `json:"id"`
	EmployeeID          string              `json:"employeeId"`
	FirstName           string              `json:"firstName"`
	SecondName          string              `json:"secondName"`
	Email               string              `json:"email"`
	Role                models.EmployeeRole `json:"role"`
	Position            string              `json:"position"`
	Team                Option              `json:"team"`
	Mentor              *MentorOption       `json:"mentor,omitempty"`
	InternshipStartDate string              `json:"internshipStartDate,omitempty"` // Календарная дата в формате YYYY-MM-DD.
}

// Option передаёт идентификатор и название команды или должности в форме.
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TeamOption содержит доступную команду, её наставников и допустимые должности.
type TeamOption struct {
	Option
	Mentors         []MentorOption `json:"mentors"`
	PositionOptions []Option       `json:"positionOptions"`
}

// MentorOption содержит данные наставника для выбора в форме.
type MentorOption struct {
	ID         string `json:"id"`
	FirstName  string `json:"firstName"`
	SecondName string `json:"secondName"`
}
