from __future__ import annotations

import asyncio
import json
import os
from types import SimpleNamespace

__version__ = "0.147.0"


class ApprovalMode:
    deny_all = "deny_all"


class Sandbox:
    read_only = "read-only"
    workspace_write = "workspace-write"


class CodexConfig:
    def __init__(
        self,
        *,
        cwd=None,
        env=None,
        experimental_api=True,
        **_kwargs,
    ):
        if experimental_api is not False:
            raise AssertionError("experimental_api must be false")
        self.cwd = cwd
        self.env = env


class AsyncCodex:
    def __init__(self, config):
        self.config = config
        self.metadata = SimpleNamespace(
            serverInfo=SimpleNamespace(version="fake-runtime-1")
        )

    async def __aenter__(self):
        return self

    async def __aexit__(self, _exc_type, _exc, _tb):
        return None

    async def account(self, *, refresh_token=False):
        assert refresh_token is False
        authenticated = os.environ.get("FAKE_CODEX_UNAUTH") != "1"
        return SimpleNamespace(
            requires_openai_auth=True,
            account=SimpleNamespace(type="fake") if authenticated else None,
        )

    async def thread_start(
        self,
        *,
        approval_mode,
        cwd,
        developer_instructions,
        ephemeral,
        sandbox,
    ):
        assert approval_mode == ApprovalMode.deny_all
        assert cwd == self.config.cwd
        assert developer_instructions
        assert ephemeral is True
        assert sandbox in {Sandbox.read_only, Sandbox.workspace_write}
        return FakeThread(cwd, sandbox)


class FakeThread:
    def __init__(self, cwd, sandbox):
        self.cwd = cwd
        self.sandbox = sandbox

    async def turn(
        self,
        prompt,
        *,
        approval_mode,
        cwd,
        output_schema,
        sandbox,
    ):
        assert approval_mode == ApprovalMode.deny_all
        assert cwd == self.cwd
        assert sandbox == self.sandbox
        return FakeTurn(prompt, output_schema)


class FakeTurn:
    def __init__(self, prompt, output_schema):
        self.prompt = prompt
        self.output_schema = output_schema
        self.interrupted = asyncio.Event()

    async def interrupt(self):
        self.interrupted.set()
        return SimpleNamespace()

    async def stream(self):
        if "TOOL_VIOLATION" in self.prompt:
            yield notification(
                "item/started",
                item=SimpleNamespace(
                    root=SimpleNamespace(type="commandExecution")
                ),
            )
            await self.interrupted.wait()
            yield terminal_notification("interrupted", "")
            return

        if self.output_schema is None:
            encoded = "Plain assistant answer."
        else:
            result = structured_result(self.output_schema)
            encoded = json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        midpoint = max(1, len(encoded) // 2)
        yield notification(
            "item/agentMessage/delta",
            delta=encoded[:midpoint],
        )
        if "BLOCK_UNTIL_CANCEL" in self.prompt:
            await self.interrupted.wait()
            yield terminal_notification("interrupted", "")
            return
        yield notification(
            "item/agentMessage/delta",
            delta=encoded[midpoint:],
        )
        yield terminal_notification("completed", encoded)


def structured_result(output_schema):
    properties = (output_schema or {}).get("properties", {})
    if "episode_notes" in properties:
        return {"episode_notes": "# Fake episode notes"}
    return {"message": "runtime-smoke-ok"}


def notification(method, **payload):
    return SimpleNamespace(method=method, payload=SimpleNamespace(**payload))


def terminal_notification(status, text):
    items = (
        [SimpleNamespace(type="agentMessage", text=text)]
        if text
        else []
    )
    turn = SimpleNamespace(status=status, items=items)
    return notification("turn/completed", turn=turn)
