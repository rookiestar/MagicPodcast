#!/usr/bin/env python3
"""Restricted one-execution Codex SDK host for MagicPodcast.

The Go parent owns execution identity and process supervision. This process
owns one SDK client, one thread, and one turn. Stdout is reserved for the
versioned JSONL protocol; diagnostics never include prompts, results, paths, or
credentials.
"""

from __future__ import annotations

import asyncio
import json
import os
import re
import signal
import sys
from contextlib import nullcontext
from dataclasses import dataclass
from pathlib import Path
from typing import Any

PROTOCOL_VERSION = 1
EXPECTED_SDK_VERSION = "0.147.0"
EXPECTED_RUNTIME_VERSION = "0.147.0"
MAX_COMMAND_BYTES = 8 << 20
ALLOWED_COMMAND_KEYS = {
    "protocol_version",
    "type",
    "execution_id",
    "kind",
    "working_directory",
    "prompt",
    "output_schema",
    "sandbox",
    "allowed_tools",
}
SAFE_ITEM_TYPES = {
    "agentMessage",
    "contextCompaction",
    "enteredReviewMode",
    "exitedReviewMode",
    "hookPrompt",
    "plan",
    "reasoning",
    "sleep",
    "userMessage",
}
TOOL_ITEM_CAPABILITIES = {
    "webSearch": "web_search",
}
DENIED_CAPABILITY_NAMES = {
    "collabAgentToolCall": "subagent",
    "commandExecution": "shell",
    "dynamicToolCall": "external_tool",
    "fileChange": "file_write",
    "imageGeneration": "image_generation",
    "imageView": "image_read",
    "mcpToolCall": "external_tool",
    "subAgentActivity": "subagent",
    "webSearch": "web_search",
}
DISABLED_RUNTIME_FEATURES = (
    "apps",
    "browser_use",
    "browser_use_external",
    "browser_use_full_cdp_access",
    "chronicle",
    "code_mode",
    "code_mode_host",
    "computer_use",
    "enable_mcp_apps",
    "executor_capability_discovery",
    "exec_permission_approvals",
    "hooks",
    "image_generation",
    "in_app_browser",
    "memories",
    "multi_agent",
    "multi_agent_v2",
    "plugins",
    "plugin_sharing",
    "recommended_plugins",
    "remote_plugin",
    "request_permissions_tool",
    "shell_snapshot",
    "shell_tool",
    "skill_mcp_dependency_install",
    "skill_search",
    "standalone_web_search",
    "tool_suggest",
    "unified_exec",
    "view_image",
)


class HostFailure(Exception):
    def __init__(self, code: str, safe_message: str) -> None:
        super().__init__(safe_message)
        self.code = code
        self.safe_message = safe_message


@dataclass(frozen=True)
class Request:
    execution_id: str
    kind: str
    working_directory: str
    prompt: str
    output_schema: dict[str, Any] | None
    sandbox: str
    allowed_tools: frozenset[str]


@dataclass(frozen=True)
class Outcome:
    status: str
    result: dict[str, Any] | None = None
    error_code: str = ""
    safe_message: str = ""


class Emitter:
    def __init__(self, execution_id: str) -> None:
        self.execution_id = execution_id
        self.sequence = 0
        self.ready = False

    def emit(self, event_type: str, **fields: Any) -> None:
        self.sequence += 1
        frame = {
            "protocol_version": PROTOCOL_VERSION,
            "execution_id": self.execution_id,
            "sequence": self.sequence,
            "type": event_type,
            **fields,
        }
        payload = json.dumps(
            frame,
            ensure_ascii=False,
            separators=(",", ":"),
        )
        sys.stdout.write(payload + "\n")
        sys.stdout.flush()
        if event_type == "ready":
            self.ready = True


class CancelState:
    def __init__(self) -> None:
        self.event = asyncio.Event()
        self.protocol_error = False

    def request(self, *, protocol_error: bool = False) -> None:
        self.protocol_error = self.protocol_error or protocol_error
        self.event.set()


async def connect_stdin() -> asyncio.StreamReader:
    loop = asyncio.get_running_loop()
    reader = asyncio.StreamReader(limit=MAX_COMMAND_BYTES)
    protocol = asyncio.StreamReaderProtocol(reader)
    await loop.connect_read_pipe(lambda: protocol, sys.stdin.buffer)
    return reader


async def read_command(reader: asyncio.StreamReader) -> dict[str, Any] | None:
    try:
        line = await reader.readline()
    except (ValueError, asyncio.LimitOverrunError) as exc:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime command exceeded protocol limits",
        ) from exc
    if line == b"":
        return None
    if len(line) > MAX_COMMAND_BYTES:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime command exceeded protocol limits",
        )
    try:
        value = json.loads(line)
    except json.JSONDecodeError as exc:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime command was not valid JSON",
        ) from exc
    if not isinstance(value, dict):
        raise HostFailure(
            "runtime_protocol_error",
            "runtime command must be a JSON object",
        )
    return value


def validate_execute(command: dict[str, Any]) -> Request:
    if set(command) - ALLOWED_COMMAND_KEYS:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime execute command contains unknown fields",
        )
    if command.get("protocol_version") != PROTOCOL_VERSION:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime protocol version is not supported",
        )
    if command.get("type") != "execute":
        raise HostFailure(
            "runtime_protocol_error",
            "runtime first command must create an execution",
        )
    execution_id = command.get("execution_id")
    kind = command.get("kind")
    working_directory = command.get("working_directory")
    prompt = command.get("prompt")
    sandbox = command.get("sandbox")
    output_schema = command.get("output_schema")
    allowed_tools = command.get("allowed_tools")
    if not isinstance(execution_id, str) or not execution_id:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime execution identity is missing",
        )
    if kind not in {"episode_notes", "assistant", "smoke"}:
        raise HostFailure(
            "invalid_request",
            "runtime execution kind is not allowed",
        )
    if not isinstance(working_directory, str):
        raise HostFailure(
            "invalid_request",
            "runtime working directory is invalid",
        )
    path = Path(working_directory)
    try:
        resolved = path.resolve(strict=True)
    except OSError as exc:
        raise HostFailure(
            "runtime_unavailable",
            "runtime working directory is unavailable",
        ) from exc
    if not path.is_absolute() or not resolved.is_dir():
        raise HostFailure(
            "invalid_request",
            "runtime working directory is invalid",
        )
    if not isinstance(prompt, str) or not prompt.strip():
        raise HostFailure(
            "invalid_request",
            "runtime prompt is invalid",
        )
    if output_schema is not None and not isinstance(output_schema, dict):
        raise HostFailure(
            "invalid_request",
            "runtime output schema is invalid",
        )
    if kind != "assistant" and output_schema is None:
        raise HostFailure(
            "invalid_request",
            "runtime output schema is required",
        )
    if sandbox not in {"read_only", "workspace_write"}:
        raise HostFailure(
            "capability_denied",
            "runtime sandbox is not allowed",
        )
    if not isinstance(allowed_tools, list) or any(
        tool not in {"web_search"} for tool in allowed_tools
    ):
        raise HostFailure(
            "capability_denied",
            "runtime tool capability is not allowed",
        )
    if len(set(allowed_tools)) != len(allowed_tools):
        raise HostFailure(
            "invalid_request",
            "runtime tool capability is duplicated",
        )
    return Request(
        execution_id=execution_id,
        kind=kind,
        working_directory=str(resolved),
        prompt=prompt,
        output_schema=output_schema,
        sandbox=sandbox,
        allowed_tools=frozenset(allowed_tools),
    )


async def watch_commands(
    reader: asyncio.StreamReader,
    request: Request,
    cancel_state: CancelState,
) -> None:
    while True:
        try:
            command = await read_command(reader)
        except HostFailure:
            cancel_state.request(protocol_error=True)
            return
        if command is None:
            cancel_state.request()
            return
        if (
            set(command)
            != {"protocol_version", "type", "execution_id"}
            or command.get("protocol_version") != PROTOCOL_VERSION
            or command.get("type") != "cancel"
            or command.get("execution_id") != request.execution_id
        ):
            cancel_state.request(protocol_error=True)
            return
        cancel_state.request()


def source_auth_file() -> Path:
    configured_home = os.environ.get("CODEX_HOME")
    source_home = (
        Path(configured_home).expanduser()
        if configured_home
        else Path.home() / ".codex"
    )
    try:
        auth_file = (source_home / "auth.json").resolve(strict=True)
    except OSError as exc:
        raise HostFailure(
            "runtime_unavailable",
            "runtime authentication is unavailable",
        ) from exc
    if not auth_file.is_file():
        raise HostFailure(
            "runtime_unavailable",
            "runtime authentication is unavailable",
        )
    return auth_file


def safe_runtime_environment(isolated_home: Path) -> dict[str, str]:
    allowed = {
        "LANG",
        "LC_ALL",
        "PATH",
        "SSL_CERT_DIR",
        "SSL_CERT_FILE",
        "TMPDIR",
    }
    environment = {
        name: os.environ[name]
        for name in allowed
        if name in os.environ
    }
    environment["CODEX_HOME"] = str(isolated_home)
    environment["HOME"] = str(isolated_home)
    environment["XDG_CONFIG_HOME"] = str(isolated_home / "config")
    return environment


def parent_owned_runtime_home() -> Path:
    configured_home = os.environ.get(
        "MAGICPODCAST_CODEX_RUNTIME_HOME",
        "",
    )
    isolated_home = Path(configured_home)
    if (
        not configured_home
        or not isolated_home.is_absolute()
        or not isolated_home.is_dir()
    ):
        raise HostFailure(
            "runtime_unavailable",
            "runtime isolation directory is unavailable",
        )
    return isolated_home


def codex_config_overrides(request: Request) -> tuple[str, ...]:
    web_search = (
        "live"
        if "web_search" in request.allowed_tools
        else "disabled"
    )
    return (
        'cli_auth_credentials_store="file"',
        f'web_search="{web_search}"',
        *(
            f"features.{feature}=false"
            for feature in DISABLED_RUNTIME_FEATURES
        ),
    )


def value_of(value: Any) -> str:
    return str(getattr(value, "value", value))


def runtime_version(metadata: Any) -> str:
    server_info = getattr(metadata, "serverInfo", None)
    version = getattr(server_info, "version", None)
    if not isinstance(version, str) or not version.strip():
        raise HostFailure(
            "runtime_unavailable",
            "runtime version could not be verified",
        )
    normalized = version.strip()
    match = re.fullmatch(
        r"(?P<version>\d+\.\d+\.\d+)(?:\s+[^\r\n]+)?",
        normalized,
    )
    if (
        match is None
        or match.group("version") != EXPECTED_RUNTIME_VERSION
    ):
        raise HostFailure(
            "runtime_unavailable",
            "runtime version is incompatible",
        )
    return f"sdk/{EXPECTED_SDK_VERSION};runtime/{normalized}"


def ensure_authenticated(account_response: Any) -> None:
    requires_auth = getattr(
        account_response,
        "requires_openai_auth",
        getattr(account_response, "requiresOpenaiAuth", True),
    )
    account = getattr(account_response, "account", None)
    if requires_auth and account is None:
        raise HostFailure(
            "runtime_unavailable",
            "runtime authentication is unavailable",
        )


def item_type(notification: Any) -> str:
    payload = getattr(notification, "payload", None)
    item = getattr(payload, "item", None)
    if item is None:
        params = getattr(payload, "params", None)
        if isinstance(params, dict):
            item = params.get("item")
    item = getattr(item, "root", item)
    if isinstance(item, dict):
        return value_of(item.get("type", ""))
    return value_of(getattr(item, "type", ""))


def enforce_tool_policy(notification: Any, request: Request) -> None:
    if getattr(notification, "method", "") != "item/started":
        return
    current_type = item_type(notification)
    if current_type in SAFE_ITEM_TYPES:
        return
    required_capability = TOOL_ITEM_CAPABILITIES.get(current_type)
    if required_capability in request.allowed_tools:
        return
    denied_capability = DENIED_CAPABILITY_NAMES.get(
        current_type,
        "unknown_tool",
    )
    raise HostFailure(
        "capability_denied",
        f"runtime requested forbidden {denied_capability} capability",
    )


def final_agent_text(turn: Any, streamed_text: str) -> str:
    items = getattr(turn, "items", None)
    if isinstance(items, list):
        for item in reversed(items):
            if value_of(getattr(item, "type", "")) == "agentMessage":
                text = getattr(item, "text", None)
                if isinstance(text, str) and text:
                    return text
    return streamed_text


def completed_outcome(
    turn: Any,
    streamed_text: str,
    request: Request,
) -> Outcome:
    status = value_of(getattr(turn, "status", ""))
    if status == "interrupted":
        return Outcome(status="cancelled")
    if status != "completed":
        return Outcome(
            status="failed",
            error_code="execution_failed",
            safe_message="runtime turn failed",
        )
    final_text = final_agent_text(turn, streamed_text)
    if request.output_schema is None:
        if not final_text.strip():
            raise HostFailure(
                "runtime_protocol_error",
                "runtime completed without an assistant result",
            )
        return Outcome(status="completed", result={"text": final_text})
    try:
        result = json.loads(final_text)
    except json.JSONDecodeError as exc:
        raise HostFailure(
            "runtime_protocol_error",
            "runtime did not return the required structured result",
        ) from exc
    if not isinstance(result, dict):
        raise HostFailure(
            "runtime_protocol_error",
            "runtime structured result is not an object",
        )
    return Outcome(status="completed", result=result)


async def consume_turn(
    turn_handle: Any,
    request: Request,
    emitter: Emitter,
    sdk_started: asyncio.Event,
    host_ready: asyncio.Event,
) -> Outcome:
    streamed_parts: list[str] = []
    async for notification in turn_handle.stream():
        if not sdk_started.is_set():
            sdk_started.set()
            await host_ready.wait()
        enforce_tool_policy(notification, request)
        method = getattr(notification, "method", "")
        payload = getattr(notification, "payload", None)
        if method == "item/agentMessage/delta":
            delta = getattr(payload, "delta", None)
            if isinstance(delta, str) and delta:
                streamed_parts.append(delta)
                emitter.emit("output_delta", text=delta)
        if method == "turn/completed":
            turn = getattr(payload, "turn", None)
            if turn is None:
                raise HostFailure(
                    "runtime_protocol_error",
                    "runtime terminal notification is invalid",
                )
            return completed_outcome(
                turn,
                "".join(streamed_parts),
                request,
            )
    raise HostFailure(
        "runtime_protocol_error",
        "runtime stream ended without a terminal result",
    )


async def run_sdk(
    request: Request,
    emitter: Emitter,
    cancel_state: CancelState,
) -> Outcome:
    try:
        import openai_codex
    except ImportError as exc:
        raise HostFailure(
            "runtime_unavailable",
            "runtime SDK is not installed",
        ) from exc

    if getattr(openai_codex, "__version__", None) != EXPECTED_SDK_VERSION:
        raise HostFailure(
            "runtime_unavailable",
            "runtime SDK version is incompatible",
        )

    sandbox = (
        openai_codex.Sandbox.read_only
        if request.sandbox == "read_only"
        else openai_codex.Sandbox.workspace_write
    )
    developer_instructions = (
        "Operate only inside the supplied working directory. "
        "Do not call a tool unless the execution policy explicitly allows it. "
        "Never inspect credentials, unrelated files, databases, backups, or "
        "other executions. Return only the requested result."
    )

    try:
        auth_file = source_auth_file()
        with nullcontext(parent_owned_runtime_home()) as isolated_home:
            try:
                (isolated_home / "auth.json").symlink_to(auth_file)
            except OSError as exc:
                raise HostFailure(
                    "runtime_unavailable",
                    "runtime authentication could not be isolated",
                ) from exc
            config = openai_codex.CodexConfig(
                config_overrides=codex_config_overrides(request),
                cwd=request.working_directory,
                env=safe_runtime_environment(isolated_home),
                experimental_api=False,
            )
            async with openai_codex.AsyncCodex(config) as client:
                account_response = await client.account(refresh_token=False)
                ensure_authenticated(account_response)
                verified_runtime_version = runtime_version(client.metadata)
                thread = await client.thread_start(
                    approval_mode=openai_codex.ApprovalMode.deny_all,
                    cwd=request.working_directory,
                    developer_instructions=developer_instructions,
                    ephemeral=True,
                    sandbox=sandbox,
                )
                turn = await thread.turn(
                    request.prompt,
                    approval_mode=openai_codex.ApprovalMode.deny_all,
                    cwd=request.working_directory,
                    output_schema=request.output_schema,
                    sandbox=sandbox,
                )
                sdk_started = asyncio.Event()
                host_ready = asyncio.Event()
                stream_task = asyncio.create_task(
                    consume_turn(
                        turn,
                        request,
                        emitter,
                        sdk_started,
                        host_ready,
                    )
                )
                sdk_started_task = asyncio.create_task(sdk_started.wait())
                done, _ = await asyncio.wait(
                    {stream_task, sdk_started_task},
                    return_when=asyncio.FIRST_COMPLETED,
                )
                if stream_task in done and not sdk_started.is_set():
                    sdk_started_task.cancel()
                    await asyncio.gather(
                        sdk_started_task,
                        return_exceptions=True,
                    )
                    return await stream_task
                await sdk_started_task
                emitter.emit(
                    "ready",
                    runtime_version=verified_runtime_version,
                )
                host_ready.set()

                cancel_task = asyncio.create_task(cancel_state.event.wait())
                done, _ = await asyncio.wait(
                    {stream_task, cancel_task},
                    return_when=asyncio.FIRST_COMPLETED,
                )
                if stream_task in done:
                    cancel_task.cancel()
                    await asyncio.gather(
                        cancel_task,
                        return_exceptions=True,
                    )
                    try:
                        return await stream_task
                    except HostFailure:
                        await turn.interrupt()
                        raise

                if cancel_state.protocol_error:
                    await turn.interrupt()
                    await asyncio.gather(
                        stream_task,
                        return_exceptions=True,
                    )
                    raise HostFailure(
                        "runtime_protocol_error",
                        "runtime cancellation command is invalid",
                    )

                try:
                    await turn.interrupt()
                except Exception as exc:
                    diagnostic = str(
                        getattr(exc, "message", "")
                    ).strip()
                    if diagnostic == "no active turn to interrupt":
                        return await stream_task
                    raise HostFailure(
                        "native_interrupt_failed",
                        (
                            "runtime native interrupt failed "
                            f"({type(exc).__name__})"
                        ),
                    ) from exc
                emitter.emit(
                    "cancel_ack",
                    cancellation_method="native_interrupt",
                )
                try:
                    outcome = await stream_task
                except HostFailure:
                    raise
                except Exception:
                    return Outcome(status="cancelled")
                if outcome.status == "completed":
                    return outcome
                return Outcome(status="cancelled")
    except HostFailure:
        raise
    except Exception as exc:
        code = "execution_failed" if emitter.ready else "runtime_unavailable"
        message = (
            "runtime execution failed"
            if emitter.ready
            else "runtime preflight failed"
        )
        raise HostFailure(code, message) from exc


async def serve() -> int:
    reader = await connect_stdin()
    command = await read_command(reader)
    if command is None:
        return 2
    request = validate_execute(command)
    emitter = Emitter(request.execution_id)
    cancel_state = CancelState()
    loop = asyncio.get_running_loop()
    for signal_name in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(signal_name, cancel_state.request)
        except (NotImplementedError, RuntimeError):
            pass

    command_task = asyncio.create_task(
        watch_commands(reader, request, cancel_state)
    )
    try:
        outcome = await run_sdk(request, emitter, cancel_state)
    except HostFailure as exc:
        outcome = Outcome(
            status="failed",
            error_code=exc.code,
            safe_message=exc.safe_message,
        )
    finally:
        command_task.cancel()
        await asyncio.gather(command_task, return_exceptions=True)

    if outcome.status == "completed":
        emitter.emit(
            "terminal",
            status="completed",
            result=outcome.result,
        )
    elif outcome.status == "cancelled":
        emitter.emit("terminal", status="cancelled")
    else:
        emitter.emit(
            "terminal",
            status="failed",
            error_code=outcome.error_code or "execution_failed",
            safe_message=outcome.safe_message or "runtime execution failed",
        )
    return 0


def main() -> int:
    try:
        return asyncio.run(serve())
    except HostFailure:
        return 2
    except Exception:
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
