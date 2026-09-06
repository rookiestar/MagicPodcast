from __future__ import annotations

from enum import Enum

__all__ = [
    "ReasoningEffort",
]


class ReasoningEffort(str, Enum):
    none = "none"
    minimal = "minimal"
    low = "low"
    medium = "medium"
    high = "high"
    xhigh = "xhigh"

    @classmethod
    def _missing_(cls, value):
        if not isinstance(value, str) or not value:
            return None
        member = str.__new__(cls, value)
        member._name_ = value
        member._value_ = value
        return member
