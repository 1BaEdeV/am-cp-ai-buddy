package mltools

type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Definitions не содержит подтверждения, решения руководителя или выдачи прав.
func Definitions() []Definition {
	return []Definition{
		{Name: "get_role_tools", Description: "Обязательные ресурсы текущего сотрудника, сохранённые статусы заявок и прогресс.", InputSchema: inputSchema("")},
		{Name: "check_tool_access", Description: "Проверить фактически выданные права на один обязательный ресурс. Не выдаёт доступ.", InputSchema: inputSchema("resourceId")},
		{Name: "prepare_access_request", Description: "Подготовить черновик одного доступа. Показать параметры пользователю и дождаться его явного подтверждения. Заявка ещё не создаётся.", InputSchema: inputSchema("resourceId")},
		{Name: "get_access_request_status", Description: "Прочитать доступную текущему сотруднику заявку по её ID, включая результат выдачи.", InputSchema: inputSchema("requestId")},
	}
}

func inputSchema(field string) map[string]any {
	properties, required := map[string]any{}, []string{}
	if field != "" {
		properties[field] = map[string]any{"type": "string", "minLength": 1}
		required = append(required, field)
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
