import importlib.util
import json
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("benchmark.py")
SPEC = importlib.util.spec_from_file_location("lsp_benchmark", SCRIPT)
benchmark = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(benchmark)


class SafeOutputTests(unittest.TestCase):
    def test_lsp_error_discards_identifying_server_text(self):
        fake_path = "/private/identifiable-corpus/secret.lua"
        response = {
            "code": -32603,
            "message": f"failed to parse {fake_path}",
            "data": {"workspace": fake_path, "revision": "deadbeef"},
        }

        encoded = json.dumps(benchmark.safe_error(response))

        self.assertEqual(
            benchmark.safe_error(response),
            {"code": -32603, "message": "LSP request failed"},
        )
        self.assertNotIn(fake_path, encoded)
        self.assertNotIn("deadbeef", encoded)

    def test_non_numeric_error_code_is_not_copied(self):
        fake_path = "/private/identifiable-corpus/secret.lua"

        result = benchmark.safe_error({"code": fake_path, "message": fake_path})

        self.assertEqual(
            result, {"code": "server_error", "message": "LSP request failed"}
        )
        self.assertNotIn(fake_path, json.dumps(result))

    def test_unknown_notification_method_is_aggregated(self):
        fake_path = "/private/identifiable-corpus/secret.lua"

        method = benchmark.safe_notification_method(fake_path)

        self.assertEqual(method, "other")
        self.assertNotIn(fake_path, json.dumps({method: 1}))
        self.assertEqual(
            benchmark.safe_notification_method("textDocument/publishDiagnostics"),
            "textDocument/publishDiagnostics",
        )


if __name__ == "__main__":
    unittest.main()
