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


class MeasurementTests(unittest.TestCase):
    def test_successful_measurement_records_result_without_errors(self):
        def request(method, params):
            return {"items": [{"label": "RageUI"}, {"label": "RageUI2"}]}, 3.25

        result, measurement = benchmark.collect_measurement(
            request,
            lambda: (128, 0.25),
            "textDocument/completion",
            {},
            1,
        )

        self.assertEqual(len(result["items"]), 2)
        self.assertEqual(measurement["result_count"], 2)
        self.assertEqual(measurement["response_bytes"], 128)
        self.assertNotIn("errors", measurement)

    def test_error_measurement_records_sanitized_error(self):
        def request(method, params):
            raise benchmark.RequestError(
                method,
                {"code": -32603, "message": "private workspace failure"},
                7.5,
            )

        result, measurement = benchmark.collect_measurement(
            request,
            lambda: (64, 0.5),
            "textDocument/hover",
            {},
            1,
        )

        self.assertIsNone(result)
        self.assertEqual(measurement["result_count"], 0)
        self.assertEqual(
            measurement["errors"],
            [{"code": -32603, "message": "LSP request failed"}],
        )


if __name__ == "__main__":
    unittest.main()
