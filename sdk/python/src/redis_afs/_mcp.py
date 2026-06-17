"""MCP-over-HTTP wire helpers (endpoint, JSON-RPC framing, result unwrap)."""

from __future__ import annotations

import json
from typing import Any, Mapping

from .errors import AFSError


def build_rpc_body(rpc_id: int, method: str, params: Mapping[str, Any] | None) -> bytes:
    return json.dumps(
        {
            "jsonrpc": "2.0",
            "id": rpc_id,
            "method": method,
            "params": dict(params or {}),
        }
    ).encode("utf-8")


def parse_rpc_payload(text: str) -> Any:
    payload = json.loads(text or "{}")
    if payload.get("error"):
        error = payload["error"]
        raise AFSError(str(error.get("message", "MCP request failed")), code=error.get("code"), payload=payload)
    return payload.get("result")


def unwrap_tool_result(result: Any, name: str) -> Any:
    if isinstance(result, dict) and result.get("isError"):
        content = "\n".join(item.get("text", "") for item in result.get("content", []))
        raise AFSError(content or f"MCP tool {name} failed", payload=result)
    if isinstance(result, dict):
        return result.get("structuredContent", result)
    return result


def normalize_mcp_endpoint(base_url: str) -> str:
    trimmed = base_url.strip().rstrip("/")
    if not trimmed:
        raise AFSError("base_url is required")
    return trimmed if trimmed.endswith("/mcp") else f"{trimmed}/mcp"


def strip_none(values: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in values.items() if value is not None}
