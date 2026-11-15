package models

import "time"

// DemoProviderAccess — фактические права в имитации внешнего сервиса.
// Пустой AccessLevel означает отсутствие доступа. EmployeeAccess обновляет
// обработчик заявки только после проверки этих прав через провайдер.
type DemoProviderAccess struct {
	EmployeeID  string `gorm:"primaryKey"`
	Employee    Employee
	Service     string    `gorm:"primaryKey"`
	ExternalID  string    `gorm:"primaryKey"`
	AccessLevel string    `gorm:"not null;default:''"`
	FailureMode string    `gorm:"not null;default:'';check:demo_provider_failure_valid,failure_mode IN ('','check_failed','grant_failed','timeout_after_grant')"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// DemoProviderOperation — успешно выполненная выдача с постоянным ID операции.
// Записывается в одной транзакции с правами. Повтор ID с другими параметрами
// запрещён; одинаковый повтор не меняет права и не выполняет выдачу заново.
type DemoProviderOperation struct {
	ID          string `gorm:"primaryKey"`
	EmployeeID  string `gorm:"not null;index"`
	Employee    Employee
	Service     string    `gorm:"not null"`
	ExternalID  string    `gorm:"not null"`
	AccessLevel string    `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null"`
}
