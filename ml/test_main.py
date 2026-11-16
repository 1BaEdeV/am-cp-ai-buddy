import os
import unittest
from unittest.mock import patch

import main


class FakeModels:
    def __init__(self):
        self.kwargs = None

    def generate_content(self, **kwargs):
        self.kwargs = kwargs
        return type("Response", (), {"text": '{"text":"  Твой наставник — Катя.  "}'})()


class FakeClient:
    def __init__(self):
        self.models = FakeModels()


def sample_payload():
    return {
        "taskId": "team-intro",
        "question": "Кто наставник?",
        "history": [{"role": "user", "text": "Кто наставник?"}],
        "context": {"teamIntro": {"mentor": {"fullName": "Катя Волкова"}}},
    }


class AgentTests(unittest.TestCase):
    def test_generate_reply_uses_context_and_history(self):
        client = FakeClient()
        result = main.generate_reply(sample_payload(), client=client)
        self.assertEqual(result, {"text": "Твой наставник — Катя."})
        self.assertEqual(client.models.kwargs["contents"][0]["parts"][0]["text"], "Кто наставник?")
        self.assertIn("Катя Волкова", client.models.kwargs["config"].system_instruction)
        self.assertEqual(client.models.kwargs["config"].response_mime_type, "application/json")

    def test_selected_team_and_people_are_passed_to_model_for_each_request(self):
        client = FakeClient()
        for team, mentor, manager in [("Поиск", "Саша Лебедев", "Дмитрий Орлов"), ("Веб-интерфейсы", "Вера Крылова", "Анна Соколова")]:
            payload = sample_payload()
            payload["context"] = {"teamIntro": {
                "team": {"name": team},
                "trainee": {"fullName": "Иван Петров", "position": "Разработчик"},
                "mentor": {"fullName": mentor, "about": "Помогает с первыми задачами", "email": "mentor@example.test"},
                "manager": {"fullName": manager, "about": "Определяет приоритеты"},
            }}
            main.generate_reply(payload, client=client)
            instruction = client.models.kwargs["config"].system_instruction
            self.assertIn(team, instruction)
            self.assertIn(mentor, instruction)
            self.assertIn(manager, instruction)
            self.assertIn("Иван Петров", instruction)
            self.assertIn("mentor@example.test", instruction)
            self.assertIn("Помогает с первыми задачами", instruction)
            other_mentor = "Вера Крылова" if mentor == "Саша Лебедев" else "Саша Лебедев"
            self.assertNotIn(other_mentor, instruction)

    def test_generate_reply_returns_access_request_intent(self):
        payload = sample_payload()
        payload["taskId"] = "working-accesses"
        payload["question"] = "Подготовь заявку на GitLab"
        client = FakeClient()
        client.models.generate_content = lambda **kwargs: type(
            "Response", (), {"text": '{"text":"Покажу черновик.","prepareAccessRequestResourceId":"gitlab-search-api"}'}
        )()
        result = main.generate_reply(payload, client=client)
        self.assertEqual(result["prepareAccessRequestResourceId"], "gitlab-search-api")

    def test_rejects_unknown_chat(self):
        payload = sample_payload()
        payload["taskId"] = "first-task"
        with self.assertRaises(main.RequestError):
            main.normalize_request(payload)

    def test_rejects_missing_context(self):
        payload = sample_payload()
        del payload["context"]
        with self.assertRaises(main.RequestError):
            main.normalize_request(payload)

    def test_requires_api_key_without_injected_client(self):
        with patch.dict(os.environ, {"GEMINI_API_KEY": ""}):
            with self.assertRaises(RuntimeError):
                main.generate_reply(sample_payload())


if __name__ == "__main__":
    unittest.main()
