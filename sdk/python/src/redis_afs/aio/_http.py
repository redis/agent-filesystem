"""Async HTTP/MCP transport: the httpx import guard and AsyncMCPHttpClient."""

from __future__ import annotations

import os
from typing import Any, Mapping

from ..errors import AFSError
from ..models import DEFAULT_BASE_URL
from .._mcp import (
    build_rpc_body,
    normalize_mcp_endpoint,
    parse_rpc_payload,
    strip_none,
    unwrap_tool_result,
)

try:
    import httpx
except ImportError:  # pragma: no cover
    httpx = None

_INSTALL_HINT = "redis-afs[async] requires httpx; install with: pip install 'redis-afs[async]'"


class AsyncMCPHttpClient:
    def __init__(
        self,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        timeout: float = 30.0,
        headers: Mapping[str, str] | None = None,
        transport: Any | None = None,
    ) -> None:
        if httpx is None:
            raise AFSError(_INSTALL_HINT)
        self.api_key = api_key or os.environ.get("AFS_API_KEY") or ""
        if not self.api_key:
            raise AFSError("AFS api_key is required")
        base = base_url or os.environ.get("AFS_API_BASE_URL") or DEFAULT_BASE_URL
        self.endpoint = normalize_mcp_endpoint(base)
        self.timeout = timeout
        self.headers = dict(headers or {})
        self._next_id = 1
        self._client = httpx.AsyncClient(timeout=timeout, transport=transport)

    async def call_tool(self, name: str, arguments: Mapping[str, Any] | None = None) -> Any:
        result = await self.request(
            "tools/call",
            {"name": name, "arguments": strip_none(dict(arguments or {}))},
        )
        return unwrap_tool_result(result, name)

    async def request(self, method: str, params: Mapping[str, Any] | None = None) -> Any:
        body = build_rpc_body(self._next_id, method, params)
        self._next_id += 1
        headers = {
            "content-type": "application/json",
            "authorization": f"Bearer {self.api_key}",
            **self.headers,
        }
        try:
            response = await self._client.post(self.endpoint, content=body, headers=headers)
        except httpx.TimeoutException as exc:
            raise AFSError(f"MCP request timed out after {self.timeout}s") from exc
        if response.status_code >= 400:
            text = response.text
            raise AFSError(
                f"MCP request failed with HTTP {response.status_code}: {text}",
                status=response.status_code,
                payload=text,
            )
        return parse_rpc_payload(response.text)

    async def aclose(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> "AsyncMCPHttpClient":
        return self

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> None:
        await self.aclose()
