"""Test parse_query against the shared query-vectors.json test vectors."""

import json
import pytest
from pathlib import Path

from hop.aim.query import parse_query
from hop.aim.types import Filter

_VECTORS_PATH = Path(__file__).parents[2] / "testdata" / "query-vectors.json"
vectors = json.loads(_VECTORS_PATH.read_text())


def _expected_to_filter(expected: dict) -> Filter:
    """Convert Go-style expected dict to a Python Filter for comparison."""
    f = Filter()
    if "Input" in expected:
        f.input = expected["Input"]
    if "Output" in expected:
        f.output = expected["Output"]
    if "Provider" in expected:
        f.provider = expected["Provider"]
    if "Family" in expected:
        f.family = expected["Family"]
    if "ToolCall" in expected:
        f.tool_call = expected["ToolCall"]
    if "Reasoning" in expected:
        f.reasoning = expected["Reasoning"]
    if "OpenWeights" in expected:
        f.open_weights = expected["OpenWeights"]
    if "StructuredOutput" in expected:
        f.structured_output = expected["StructuredOutput"]
    if "Temperature" in expected:
        f.temperature = expected["Temperature"]
    if "Query" in expected:
        f.query = expected["Query"]
    return f


@pytest.mark.parametrize("v", vectors, ids=[v["description"] for v in vectors])
def test_vector(v: dict) -> None:
    if v.get("error"):
        with pytest.raises(ValueError):
            parse_query(v["input"])
    else:
        got = parse_query(v["input"])
        want = _expected_to_filter(v.get("expected") or {})
        assert got == want, f"got {got!r}, want {want!r}"
