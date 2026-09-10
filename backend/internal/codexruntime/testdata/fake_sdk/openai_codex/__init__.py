from __future__ import annotations

import asyncio
import json
import os
import signal
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
        bridge = "true" if 'web_search="live"' in self.config_overrides else "false"
        if f"features.code_mode_host={bridge}" not in self.config_overrides:
            raise AssertionError("web bridge must match the allowed web capability")
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

    async def models(self, *, include_hidden=False):
        assert include_hidden is False
        return _fake_model_catalog()

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


def _fake_model_catalog():
    """Model catalog shaped like the SDK's ModelListResponse.

    FAKE_CODEX_MODEL_CATALOG overrides the default with a JSON list of
    {"model": ..., "efforts": [...], "tiers": [...], "default_tier": ...}
    entries so tests can simulate accounts that lack a requested profile or
    would make Standard inherit Fast.
    """
    if os.environ.get("FAKE_CODEX_MODEL_CATALOG_INVALID") == "1":
        return SimpleNamespace(data={"invalid": "model catalog shape"})
    encoded = os.environ.get("FAKE_CODEX_MODEL_CATALOG")
    if encoded:
        entries = json.loads(encoded)
    else:
        entries = [
            {
                "model": "gpt-5.6-luna",
                "efforts": ["medium", "max"],
                "tiers": ["priority"],
            },
            {
                "model": "gpt-5.6-sol",
                "efforts": ["medium", "xhigh"],
                "tiers": ["priority"],
            },
        ]
    data = []
    for entry in entries:
        data.append(
            SimpleNamespace(
                model=entry["model"],
                id=entry.get("id", entry["model"]),
                supported_reasoning_efforts=[
                    SimpleNamespace(reasoning_effort=effort)
                    for effort in entry["efforts"]
                ],
                service_tiers=[
                    SimpleNamespace(id=tier) for tier in entry["tiers"]
                ],
                default_service_tier=entry.get("default_tier"),
            )
        )
    return SimpleNamespace(data=data)


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
        model=None,
        effort=None,
        service_tier=None,
    ):
        from .types import ReasoningEffort

        assert approval_mode == ApprovalMode.deny_all
        assert cwd == self.cwd
        assert sandbox == self.sandbox
        if model is not None:
            assert isinstance(model, str) and model
        if effort is not None:
            assert isinstance(effort, ReasoningEffort)
        if service_tier is not None:
            assert isinstance(service_tier, str) and service_tier
        return FakeTurn(
            prompt,
            output_schema,
            cwd,
            self.config,
            model,
            effort,
            service_tier,
        )


class FakeTurn:
    def __init__(
        self,
        prompt,
        output_schema,
        cwd,
        config,
        model=None,
        effort=None,
        service_tier=None,
    ):
        self.prompt = prompt
        self.output_schema = output_schema
        self.cwd = cwd
        self.config = config
        self.model = model
        self.effort = effort
        self.service_tier = service_tier
        self.interrupted = asyncio.Event()
        self.record_turn_parameters()

    def record_turn_parameters(self):
        """Persist the per-turn parameters where tests can observe them."""
        payload = {
            "model": self.model,
            "effort": (
                str(getattr(self.effort, "value", self.effort))
                if self.effort is not None
                else None
            ),
            "service_tier": self.service_tier,
        }
        try:
            Path(self.cwd, "fake-turn-params.json").write_text(
                json.dumps(payload, ensure_ascii=False, sort_keys=True),
                encoding="utf-8",
            )
        except OSError:
            pass

    async def interrupt(self):
        self.interrupted.set()
        return SimpleNamespace()

    async def stream(self):
        if "HANG_FOR_SIGKILL" in self.prompt:
            signal.signal(signal.SIGTERM, signal.SIG_IGN)
            yield notification(
                "item/agentMessage/delta",
                delta="runtime started",
            )
            await asyncio.Event().wait()

        if "PROGRESS_FLOW" in self.prompt:
            for note in progress_flow_notifications(self):
                yield note
            if "BLOCK_UNTIL_CANCEL" in self.prompt:
                await self.interrupted.wait()
                yield terminal_notification("interrupted", "")
                return
            yield terminal_notification("completed", "Progress answer.")
            return

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
        if "ITEM_COMPLETED_ONLY" in self.prompt:
            yield notification(
                "item/completed",
                item=SimpleNamespace(
                    root=SimpleNamespace(type="agentMessage", text=encoded)
                ),
            )
            yield terminal_notification("completed", "")
            return
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


def progress_flow_notifications(self):
    """Exercise every sanitized activity mapping the host must support."""
    from types import SimpleNamespace

    managed_secret = os.path.join(self.cwd, "managed-notes.txt")
    yield notification("turn/started", turn=SimpleNamespace(id="t1"))
    yield notification(
        "item/started",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="webSearch",
                id="ws-1",
                query=(
                    "episode contract  \n long topic "
                    + "x" * 400
                    + " secret="
                    + managed_secret
                ),
                results=None,
            )
        ),
    )
    yield notification(
        "item/completed",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="webSearch",
                id="ws-1",
                query="episode contract",
                results=[
                    {
                        "url": "https://Authority.example.com/a/path?q=1",
                        "title": "A",
                    },
                    {"url": "https://user:leak@bad.example.com/x"},
                    {"url": "file:///etc/hosts"},
                    {"title": "no url"},
                    "not-an-object",
                ],
            )
        ),
    )
    yield notification(
        "item/started",
        item=SimpleNamespace(
            root=SimpleNamespace(type="reasoning", id="r-1")
        ),
    )
    for index in range(6):
        yield notification(
            "item/reasoning/summaryTextDelta",
            item_id="r-1",
            summary_index=0,
            delta=f"summary part {index}; ",
        )
    yield notification(
        "item/reasoning/textDelta",
        item_id="r-1",
        delta="RAW CHAIN OF THOUGHT MUST NOT LEAK",
    )
    yield notification(
        "item/completed",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="reasoning",
                id="r-1",
                summary=["Final visible summary."],
                content=["PRIVATE MODEL THOUGHTS"],
            )
        ),
    )
    yield notification(
        "item/plan/delta",
        item_id="p-1",
        delta="先核对来源，再回答。",
    )
    yield notification(
        "item/completed",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="plan",
                id="p-1",
                text="先核对来源，再回答。",
            )
        ),
    )
    # Unknown notifications must be ignored without breaking the stream.
    yield notification(
        "item/mcpToolCall/progress",
        item_id="x-1",
        progress="should never reach progress frames",
    )
    yield notification(
        "turn/diff/updated",
        diff="should never reach progress frames",
    )
    yield notification(
        "item/started",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="agentMessage", id="m-1", phase="final_answer"
            )
        ),
    )
    midpoint = len("Final answer text.") // 2
    yield notification(
        "item/agentMessage/delta",
        item_id="m-1",
        delta="Final answer text."[:midpoint],
    )
    yield notification(
        "item/agentMessage/delta",
        item_id="m-1",
        delta="Final answer text."[:midpoint],
    )
    yield notification(
        "item/completed",
        item=SimpleNamespace(
            root=SimpleNamespace(
                type="agentMessage",
                id="m-1",
                phase="final_answer",
                text="Final answer text.",
            )
        ),
    )


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
