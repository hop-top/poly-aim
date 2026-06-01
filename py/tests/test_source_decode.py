"""Wire-format decode parity: every Model field round-trips from JSON."""

from hop.aim.source import _model_from_dict


def test_decode_full_model():
    raw = {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "family": "gpt-4",
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "tool_call": True,
        "reasoning": False,
        "open_weights": False,
        "attachment": True,
        "cost": {
            "input": 5.0,
            "output": 15.0,
            "cache_read": 1.25,
            "cache_write": 6.25,
        },
        "structured_output": True,
        "temperature": True,
        "release_date": "2024-05-13",
        "last_updated": "2024-05-13",
        "knowledge": "2023-10",
        "limit": {"context": 128000, "input": 128000, "output": 4096},
    }
    m = _model_from_dict(raw, "openai")
    assert m.id == "gpt-4o"
    assert m.provider == "openai"
    assert m.tool_call is True
    assert m.reasoning is False
    assert m.attachment is True
    assert m.cost is not None
    assert m.cost.input == 5.0
    assert m.cost.output == 15.0
    assert m.cost.cache_read == 1.25
    assert m.cost.cache_write == 6.25
    assert m.structured_output is True
    assert m.temperature is True
    assert m.modalities.input == ["text", "image"]
    assert m.limit.context == 128000


def test_decode_cost_partial():
    """Absent cost fields decode as None (not 0.0)."""
    raw = {
        "id": "x", "name": "X",
        "modalities": {"input": [], "output": []},
        "cost": {"input": 1.5},  # only input set
        "limit": {},
    }
    m = _model_from_dict(raw, "p")
    assert m.cost is not None
    assert m.cost.input == 1.5
    assert m.cost.output is None
    assert m.cost.cache_read is None
    assert m.cost.cache_write is None


def test_decode_cost_absent():
    raw = {
        "id": "llama3",
        "name": "Llama 3",
        "modalities": {"input": ["text"], "output": ["text"]},
        "tool_call": False,
        "reasoning": False,
        "open_weights": True,
        "limit": {"context": 8192},
    }
    m = _model_from_dict(raw, "ollama")
    assert m.cost is None, "cost absent in wire → Model.cost is None (not Cost(0,0,0,0))"
    assert m.structured_output is False
    assert m.temperature is False


def test_decode_new_bools_default_false():
    raw = {"id": "x", "name": "X", "modalities": {"input": [], "output": []}, "limit": {}}
    m = _model_from_dict(raw, "p")
    assert m.structured_output is False
    assert m.temperature is False
    assert m.cost is None
