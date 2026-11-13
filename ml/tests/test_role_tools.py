import unittest

from role_tools import build_role_tools, load_mock_resources


class RoleToolsTests(unittest.TestCase):
    def test_short_resource_id(self):
        _, check = build_role_tools(lambda: load_mock_resources("intern_backend"))
        expected = check("gitlab", "gitlab-search-api")
        self.assertEqual(expected["resources"][0]["status"], "requestable")
        self.assertEqual(check("gitlab", "search-api"), expected)

        _, check = build_role_tools(lambda: load_mock_resources("intern_frontend"))
        self.assertEqual(check("gitlab", "search-api"), {"error": "resource_not_found"})
    def test_demo_profiles(self):
        cases = (
            ("intern_ml", "jupyter", "jupyter-training", "granted", False),
            ("intern_ml", "s3", "s3-training", "requestable", True),
            ("intern_backend", "gitlab", "gitlab-search-api", "requestable", True),
            ("intern_frontend", "gitlab", "gitlab-onboarding", "granted", False),
        )
        for user_id, tool_id, resource_id, status, can_request in cases:
            with self.subTest(user_id=user_id, resource_id=resource_id):
                list_tools, check = build_role_tools(
                    lambda: load_mock_resources(user_id)
                )
                result = check(tool_id, resource_id)
                self.assertEqual(len(result["resources"]), 1)
                resource = result["resources"][0]
                self.assertEqual(resource["id"], resource_id)
                self.assertEqual(resource["status"], status)
                self.assertEqual(resource["canRequest"], can_request)
                self.assertIn(resource, list_tools()["resources"])
                self.assertEqual(check(tool_id), result)

    def test_resource_outside_profile(self):
        _, check = build_role_tools(lambda: load_mock_resources("intern_frontend"))
        self.assertEqual(
            check("gitlab", "gitlab-search-api"),
            {"error": "resource_not_found"},
        )

    def test_unknown_profile(self):
        list_tools, check = build_role_tools(lambda: load_mock_resources("unknown"))
        expected = {"error": "intern_not_found"}
        self.assertEqual(list_tools(), expected)
        self.assertEqual(check("gitlab"), expected)