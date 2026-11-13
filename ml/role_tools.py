RESOURCES = {
    "team-wiki": ("wiki", "Вики команды", "read", "Инструкции команды"),
    "youtrack-search": ("youtrack", "YouTrack", "member", "Учебные задачи"),
    "jupyter-training": ("jupyter", "Jupyter", "user", "Эксперименты с моделями"),
    "s3-training": ("s3", "Учебный S3", "read", "Учебные датасеты"),
    "gitlab-search-api": ("gitlab", "GitLab · search-api", "Developer", "Код поиска"),
    "grafana-search": ("grafana", "Grafana · Поиск", "Viewer", "Метрики поиска"),
    "gitlab-onboarding": ("gitlab", "GitLab · onboarding", "Developer", "Код интерфейса"),
    "figma-design": ("figma", "Figma", "Viewer", "Макеты интерфейса"),
}

DEMO_ACCESS = {
    "intern_ml": {
        "team-wiki": "granted",
        "youtrack-search": "granted",
        "jupyter-training": "granted",
        "s3-training": "requestable",
    },
    "intern_backend": {
        "team-wiki": "granted",
        "youtrack-search": "granted",
        "gitlab-search-api": "requestable",
        "grafana-search": "requestable",
    },
    "intern_frontend": {
        "team-wiki": "granted",
        "youtrack-search": "granted",
        "gitlab-onboarding": "granted",
        "figma-design": "requestable",
    },
}


def load_mock_resources(user_id: str) -> list[dict] | None:
    access = DEMO_ACCESS.get(user_id)
    if access is None:
        return None

    resources = []
    for resource_id, status in access.items():
        service, name, level, purpose = RESOURCES[resource_id]
        resources.append({
            "id": resource_id,
            "service": service,
            "name": name,
            "accessLevel": level,
            "purpose": purpose,
            "status": status,
            "canRequest": status == "requestable",
        })
    return resources


def build_role_tools(load_resources):
    def get_role_tools() -> dict:
        print("[TOOL CALL] get_role_tools")
        resources = load_resources()
        if resources is None:
            return {"error": "intern_not_found"}
        return {"resources": resources}

    def check_tool_access(tool_id: str, resource_id: str | None = None) -> dict:
        print("[TOOL CALL] check_tool_access")
        resources = load_resources()
        if resources is None:
            return {"error": "intern_not_found"}

        service_resources = [
            resource for resource in resources if resource["service"] == tool_id
        ]
        if resource_id is None:
            return (
                {"resources": service_resources}
                if service_resources
                else {"error": "resource_not_found"}
            )

        matches = [
            resource for resource in service_resources if resource["id"] == resource_id
        ]
        if not matches:
            full_id = f"{tool_id}-{resource_id}"
            matches = [
                resource for resource in service_resources if resource["id"] == full_id
            ]

        if not matches:
            return {"error": "resource_not_found"}
        return {"resources": matches}

    return [get_role_tools, check_tool_access]
