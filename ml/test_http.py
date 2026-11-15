import json
import unittest
from http.client import HTTPConnection
from http.server import ThreadingHTTPServer
from threading import Thread
from unittest.mock import patch

import main
from test_main import sample_payload


class HTTPTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        server = ThreadingHTTPServer(("127.0.0.1", 0), main.Handler)
        cls.port = server.server_port
        cls.addClassCleanup(server.server_close)
        thread = Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01})
        thread.start()
        cls.addClassCleanup(thread.join)
        cls.addClassCleanup(server.shutdown)

    def request(self, method, path, body=None, headers=None):
        connection = HTTPConnection("127.0.0.1", self.port, timeout=5)
        try:
            connection.request(method, path, body=body, headers=headers or {})
            response = connection.getresponse()
            return response.status, json.load(response)
        finally:
            connection.close()

    def test_health_and_unknown_routes(self):
        with patch.dict("os.environ", {"GEMINI_API_KEY": ""}):
            self.assertEqual(self.request("GET", "/health"), (200, {
                "status": "ok",
                "model": main.MODEL,
                "modelConfigured": False,
            }))
        for method in ("GET", "POST"):
            self.assertEqual(
                self.request(method, "/unknown"), (404, {"error": "not_found"})
            )

    def test_chat_rejects_invalid_requests(self):
        for body, headers in (
            (b"", {}),
            (b"{", {}),
            (b"[]", {}),
            (b"\xff", {}),
            (b"", {"Content-Length": "invalid"}),
            (b"", {"Content-Length": str(main.MAX_BODY_BYTES + 1)}),
        ):
            with self.subTest(body=body, headers=headers):
                status, result = self.request("POST", "/chat", body, headers)
                self.assertEqual(status, 400)
                self.assertEqual(result["error"], "invalid_request")

    def test_chat_returns_model_answer(self):
        payload = sample_payload()
        with patch.object(main, "generate_reply", return_value={"text": "Ответ"}) as reply:
            self.assertEqual(
                self.request("POST", "/chat", json.dumps(payload)),
                (200, {"text": "Ответ"}),
            )
        reply.assert_called_once_with(payload)

    def test_chat_reports_unconfigured_model(self):
        with patch.dict("os.environ", {"GEMINI_API_KEY": ""}):
            status, result = self.request("POST", "/chat", json.dumps(sample_payload()))
        self.assertEqual(status, 503)
        self.assertEqual(result["error"], "agent_unavailable")

    def test_chat_hides_provider_error(self):
        with (
            patch.object(main, "generate_reply", side_effect=OSError("internal detail")),
            self.assertLogs(level="ERROR"),
        ):
            status, result = self.request("POST", "/chat", json.dumps(sample_payload()))
        self.assertEqual(status, 502)
        self.assertEqual(result["error"], "model_error")
        self.assertNotIn("internal detail", json.dumps(result))