package auth

import (
	"errors"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/models"
)

var (
	ErrInvalidInput            = errors.New("проверь данные формы")
	ErrEmployeeNotFound        = errors.New("сотрудник не найден")
	ErrForbidden               = errors.New("роль сотрудника не поддерживается")
	ErrRegistrationUnavailable = errors.New("справочники для регистрации не заполнены")
	ErrProfileAlreadyExists    = errors.New("профиль уже сохранён с другими данными")
)

type LoginStatus string

const (
	LoginReady             LoginStatus = "ready"
	LoginNeedsRegistration LoginStatus = "registration_required"
)

type LoginResult struct {
	Status          LoginStatus         `json:"status"`
	Profile         *Profile            `json:"profile,omitempty"`
	EmployeeID      string              `json:"employeeId,omitempty"`
	FullName        string              `json:"fullName,omitempty"`
	Email           string              `json:"email,omitempty"`
	Role            models.EmployeeRole `json:"role,omitempty"`
	PositionID      string              `json:"positionId,omitempty"`
	TeamOptions     []TeamOption        `json:"teamOptions,omitempty"`
	PositionOptions []Option            `json:"positionOptions,omitempty"`
}

type RegistrationInput struct {
	TeamID              string `json:"teamId"`
	PositionID          string `json:"positionId,omitempty"`
	MentorID            string `json:"mentorId,omitempty"`
	InternshipStartDate string `json:"internshipStartDate,omitempty"`
}

type Profile struct {
	ID                  string              `json:"id"`
	EmployeeID          string              `json:"employeeId"`
	FullName            string              `json:"fullName"`
	Email               string              `json:"email"`
	Role                models.EmployeeRole `json:"role"`
	Position            string              `json:"position"`
	Team                Option              `json:"team"`
	InternshipStartDate string              `json:"internshipStartDate,omitempty"`
}

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TeamOption struct {
	Option
	Mentors []MentorOption `json:"mentors"`
}

type MentorOption struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}
