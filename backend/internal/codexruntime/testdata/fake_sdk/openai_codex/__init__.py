from __future__ import annotations

import asyncio
import json
import os
from pathlib import Path
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
        config_overrides=(),
        **_kwargs,
    ):
        if experimental_api is not False:
            raise AssertionError("experimental_api must be false")
        self.cwd = cwd
        self.env = env
        self.config_overrides = tuple(config_overrides)
        required_overrides = {
            'cli_auth_credentials_store="file"',
            "features.apps=false",
            "features.hooks=false",
            "features.image_generation=false",
            "features.multi_agent=false",
            "features.plugins=false",
            "features.shell_tool=false",
            "features.skill_search=false",
            "features.tool_suggest=false",
            "features.unified_exec=false",
            "features.view_image=false",
        }
        if not required_overrides.issubset(self.config_overrides):
            raise AssertionError("restricted feature overrides are missing")
        if not {
            'web_search="disabled"',
            'web_search="live"',
        }.intersection(self.config_overrides):
            raise AssertionError("web search mode is not explicit")
        isolated_home = Path(self.env["CODEX_HOME"])
        source_home = Path(os.environ["CODEX_HOME"])
        if isolated_home == source_home:
            raise AssertionError("CODEX_HOME must be isolated")
        if self.env["HOME"] != str(isolated_home):
            raise AssertionError("HOME must be isolated")
        if not (isolated_home / "auth.json").is_symlink():
            raise AssertionError("isolated auth link is missing")


class AsyncCodex:
    def __init__(self, config):
        self.config = config
        self.metadata = SimpleNamespace(
            serverInfo=SimpleNamespace(
                version=os.environ.get(
                    "FAKE_CODEX_RUNTIME_VERSION",
                    (
                        "0.147.0 (Mac OS 26.6.1; arm64) "
                        "unknown (codex_python_sdk; 0.147.0)"
                    ),
                )
            )
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
        return FakeThread(cwd, sandbox, self.config)


class FakeThread:
    def __init__(self, cwd, sandbox, config):
        self.cwd = cwd
        self.sandbox = sandbox
        self.config = config

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
        return FakeTurn(prompt, output_schema, self.cwd, self.config)


class FakeTurn:
    def __init__(self, prompt, output_schema, cwd, config):
        self.prompt = prompt
        self.output_schema = output_schema
        self.cwd = cwd
        self.config = config
        self.interrupted = asyncio.Event()

    async def interrupt(self):
        self.interrupted.set()
        return SimpleNamespace()

    async def stream(self):
        if "FORCED_TOOL_EVENT" in self.prompt:
            yield notification(
                "item/started",
                item=SimpleNamespace(
                    root=SimpleNamespace(type="commandExecution")
                ),
            )
            await self.interrupted.wait()
            yield terminal_notification("interrupted", "")
            return

        if "TOOL_VIOLATION" in self.prompt:
            shell_enabled = (
                "features.shell_tool=false"
                not in self.config.config_overrides
            )
            if shell_enabled:
                Path(self.cwd, "forbidden-tool-dispatched").write_text(
                    "dispatched",
                    encoding="utf-8",
                )
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
