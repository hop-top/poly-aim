"""Types mirroring the Go aim package types."""

from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Modalities:
    input: list[str] = field(default_factory=list)
    output: list[str] = field(default_factory=list)


@dataclass
class Limits:
    context: Optional[int] = None
    input: Optional[int] = None
    output: Optional[int] = None


@dataclass
class Cost:
    """Per-token pricing in USD per 1M tokens. All fields optional.

    Fields are Optional[float]: None distinguishes "field absent in wire
    payload" from explicit 0.0. Open-weight/local models typically omit
    cost fields entirely; consumers checking `cost.input is None` can
    detect "unknown" vs "free".
    """
    input: Optional[float] = None
    output: Optional[float] = None
    cache_read: Optional[float] = None
    cache_write: Optional[float] = None


@dataclass
class Model:
    id: str = ""
    name: str = ""
    family: Optional[str] = None
    # Backfilled from parent Provider.id during deserialization.
    provider: str = ""
    modalities: Modalities = field(default_factory=Modalities)
    tool_call: bool = False
    reasoning: bool = False
    open_weights: bool = False
    attachment: Optional[bool] = None
    cost: Optional[Cost] = None
    structured_output: bool = False
    temperature: bool = False
    release_date: Optional[str] = None
    last_updated: Optional[str] = None
    knowledge: Optional[str] = None
    limit: Limits = field(default_factory=Limits)


@dataclass
class Provider:
    id: str = ""
    name: str = ""
    doc: Optional[str] = None
    api: Optional[str] = None
    npm: Optional[str] = None
    env: list[str] = field(default_factory=list)
    models: dict[str, Model] = field(default_factory=dict)


@dataclass
class Filter:
    """Constrains a model query. All set fields are ANDed.

    input/output: subset match against model modalities.
    tool_call/reasoning/open_weights/structured_output/temperature:
    tristate — None = no filter, True/False = must match.
    """
    input: list[str] = field(default_factory=list)
    output: list[str] = field(default_factory=list)
    provider: str = ""
    family: str = ""
    tool_call: Optional[bool] = None   # tristate
    reasoning: Optional[bool] = None
    open_weights: Optional[bool] = None
    structured_output: Optional[bool] = None   # tristate
    temperature: Optional[bool] = None         # tristate
    query: str = ""
