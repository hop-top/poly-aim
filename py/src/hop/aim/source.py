"""ModelsDevSource — fetches provider data from models.dev/api.json."""

from __future__ import annotations

from dataclasses import fields, is_dataclass
from types import UnionType
from typing import Any, Union, get_args, get_origin, get_type_hints

import httpx

from .types import Model, Provider

DEFAULT_SOURCE_URL = "https://models.dev/api.json"
DEFAULT_MAX_RESPONSE_SIZE = 50 * 1024 * 1024  # 50 MB


def _unwrap_optional(tp: Any) -> Any:
    """Strip Optional[X] / X | None to X. Pass-through for other types."""
    if get_origin(tp) in (Union, UnionType):
        non_none = [a for a in get_args(tp) if a is not type(None)]
        if len(non_none) == 1:
            return non_none[0]
    return tp


def _decode(value: Any, tp: Any) -> Any:
    """Recursively decode `value` against the annotated type `tp`.

    Handles dataclasses, Optional, list[X], dict[K, V]. Leaves scalars
    (str/int/float/bool) and unknown types untouched — JSON already
    produces the right Python primitives.
    """
    if value is None:
        return None
    inner = _unwrap_optional(tp)
    origin = get_origin(inner)
    if is_dataclass(inner):
        return _from_dict(inner, value)
    if origin is list:
        (item_tp,) = get_args(inner) or (Any,)
        return [_decode(v, item_tp) for v in value]
    if origin is dict:
        _, val_tp = get_args(inner) or (Any, Any)
        return {k: _decode(v, val_tp) for k, v in value.items()}
    return value


def _from_dict(cls: type, d: dict) -> Any:
    """Build a dataclass instance from a wire dict.

    Fields not present in `d` fall through to the dataclass default,
    so absent bools stay `False`, absent Optionals stay `None`.
    """
    hints = get_type_hints(cls)
    kwargs: dict[str, Any] = {}
    for f in fields(cls):
        if f.name not in d:
            continue
        kwargs[f.name] = _decode(d[f.name], hints[f.name])
    return cls(**kwargs)


def _model_from_dict(d: dict, provider_id: str) -> Model:
    m = _from_dict(Model, d)
    m.provider = provider_id
    return m


def _provider_from_dict(d: dict, key: str) -> Provider:
    provider_id = d.get("id") or key
    p = _from_dict(Provider, {**d, "id": provider_id})
    for model_key, model in p.models.items():
        if model is not None:
            model.provider = provider_id
    # Strip any None model values (wire payload may carry them).
    p.models = {k: v for k, v in p.models.items() if v is not None}
    return p


class ModelsDevSource:
    """Fetches provider data from models.dev/api.json using httpx."""

    def __init__(
        self,
        url: str = DEFAULT_SOURCE_URL,
        max_response_size: int = DEFAULT_MAX_RESPONSE_SIZE,
    ) -> None:
        self.url = url
        self.max_response_size = max_response_size

    async def fetch(self) -> dict[str, Provider]:
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                self.url,
                headers={"Accept": "application/json"},
                follow_redirects=True,
            )

        if resp.status_code != 200:
            raise RuntimeError(
                f"aim: fetch {self.url}: unexpected status {resp.status_code}"
            )

        raw_bytes = resp.content
        if len(raw_bytes) > self.max_response_size:
            raise RuntimeError(
                f"aim: response from {self.url} exceeds max size "
                f"({self.max_response_size} bytes)"
            )

        raw: dict = resp.json()

        providers: dict[str, Provider] = {}
        for key, pdata in raw.items():
            if not pdata:
                continue
            p = _provider_from_dict(pdata, key)
            if p.id != key:
                raise RuntimeError(
                    f'aim: provider map key "{key}" != provider id "{p.id}"'
                )
            providers[key] = p

        return providers
