"""Registry — async model registry backed by models.dev."""

from __future__ import annotations

from typing import Optional

from .query import parse_query
from .source import ModelsDevSource
from .types import Filter, Model, Provider

# NOTE: XDG cache is used in this Python port via platformdirs (optional dep).
# If platformdirs is not installed, in-memory cache is used instead.
# See testdata/xdg.md for XDG cache dir algorithm.

try:
    from platformdirs import user_cache_dir as _user_cache_dir
    _HAS_PLATFORMDIRS = True
except ImportError:
    _HAS_PLATFORMDIRS = False


def _xdg_cache_dir() -> Optional[str]:
    """Return XDG-compliant cache dir for hop/aim, or None if unavailable."""
    import os
    xdg = os.environ.get("XDG_CACHE_HOME")
    if xdg:
        return os.path.join(xdg, "hop", "aim")
    if _HAS_PLATFORMDIRS:
        return os.path.join(_user_cache_dir("hop", appauthor=False), "aim")
    return None


def _matches_filter(m: Model, f: Filter) -> bool:
    if f.input:
        for mod in f.input:
            if mod not in m.modalities.input:
                return False
    if f.output:
        for mod in f.output:
            if mod not in m.modalities.output:
                return False
    if f.provider and m.provider != f.provider:
        return False
    if f.family and m.family != f.family:
        return False
    if f.tool_call is not None and m.tool_call != f.tool_call:
        return False
    if f.reasoning is not None and m.reasoning != f.reasoning:
        return False
    if f.open_weights is not None and m.open_weights != f.open_weights:
        return False
    if f.structured_output is not None and m.structured_output != f.structured_output:
        return False
    if f.temperature is not None and m.temperature != f.temperature:
        return False
    if f.query:
        q = f.query.lower()
        if q not in m.id.lower() and q not in m.name.lower():
            return False
    return True


class Registry:
    """Async registry providing access to models.dev provider/model data."""

    def __init__(self, url: Optional[str] = None) -> None:
        self._source = ModelsDevSource(url=url) if url else ModelsDevSource()
        self._cache: Optional[dict[str, Provider]] = None

    async def refresh(self) -> None:
        """Force re-fetch from source."""
        self._cache = await self._source.fetch()

    async def _load(self) -> dict[str, Provider]:
        if self._cache is None:
            self._cache = await self._source.fetch()
        return self._cache

    async def providers(self) -> list[Provider]:
        data = await self._load()
        return list(data.values())

    async def models(self, filter: Optional[Filter] = None) -> list[Model]:
        data = await self._load()
        all_models: list[Model] = []
        for p in data.values():
            all_models.extend(p.models.values())
        if filter is None:
            return all_models
        return [m for m in all_models if _matches_filter(m, filter)]

    async def get(self, provider_id: str, model_id: str) -> Optional[Model]:
        data = await self._load()
        p = data.get(provider_id)
        if p is None:
            return None
        return p.models.get(model_id)

    async def query(self, q: str) -> list[Model]:
        f = parse_query(q)
        return await self.models(f)
