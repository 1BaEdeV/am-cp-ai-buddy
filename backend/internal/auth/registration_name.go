package auth

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

type employeeName struct {
	first, second, middle string
}

func registrationName(value *string, employee models.Employee) (employeeName, error) {
	if value == nil {
		return employeeName{employee.FirstName, employee.SecondName, employee.MiddleName}, nil
	}
	parts := strings.Fields(*value)
	if len(parts) < 2 || len(parts) > 3 || utf8.RuneCountInString(*value) > 150 {
		return employeeName{}, fmt.Errorf("%w: укажи фамилию, имя и отчество, если есть", ErrInvalidInput)
	}
	for _, part := range parts {
		letters := false
		for _, r := range part {
			if unicode.IsLetter(r) {
				letters = true
			} else if !unicode.IsMark(r) && r != '-' && r != '\'' && r != '’' && r != 'ʼ' {
				return employeeName{}, fmt.Errorf("%w: ФИО может содержать буквы, дефис и апостроф", ErrInvalidInput)
			}
		}
		if !letters {
			return employeeName{}, fmt.Errorf("%w: укажи фамилию и имя полностью", ErrInvalidInput)
		}
	}
	name := employeeName{first: parts[1], second: parts[0]}
	if len(parts) == 3 {
		name.middle = parts[2]
	}
	return name, nil
}
