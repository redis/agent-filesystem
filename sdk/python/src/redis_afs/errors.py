"""Public exception types for the redis_afs SDK."""

from __future__ import annotations

from typing import Any


class AFSError(RuntimeError):
    def __init__(
        self,
        message: str,
        *,
        status: int | None = None,
        code: int | None = None,
        payload: Any | None = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.code = code
        self.payload = payload
