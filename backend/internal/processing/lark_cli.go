package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const maxLarkCLIOutputBytes = 32 << 20

var errLarkMinutesPending = errors.New("Feishu Minutes is still processing")

type larkCommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execLarkCLI struct {
	command string
}

type larkCommandError struct {
	exitCode int
	stdout   []byte
	stderr   []byte
	cause    error
}

func (e *larkCommandError) Error() string {
	if e == nil {
		return ""
	}
	return "lark-cli command failed"
}

func newExecLarkCLI(command string) (*execLarkCLI, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "lark-cli"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, NewAdapterError(
			"lark_cli_unavailable",
			"Feishu CLI is unavailable",
			true,
		)
	}
	return &execLarkCLI{command: path}, nil
}

func (r *execLarkCLI) Run(
	ctx context.Context,
	workingDirectory string,
	args ...string,
) ([]byte, error) {
	if !filepathIsCanonicalDirectory(workingDirectory) {
		return nil, NewAdapterError(
			"lark_workdir_unavailable",
			"Feishu CLI working directory is unavailable",
			false,
		)
	}
	command := exec.CommandContext(ctx, r.command, args...)
	command.Dir = workingDirectory
	command.Env = append(
		os.Environ(),
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
	)
	var stdout, stderr limitedBuffer
	stdout.limit = maxLarkCLIOutputBytes
	stderr.limit = maxLarkCLIOutputBytes
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil && (stdout.overflow || stderr.overflow) {
		return nil, NewAdapterError(
			"lark_protocol_error",
			"Feishu CLI output exceeded the safe limit",
			false,
		)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return nil, &larkCommandError{
			exitCode: exitCode,
			stdout:   stdout.Bytes(),
			stderr:   stderr.Bytes(),
			cause:    err,
		}
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(input []byte) (int, error) {
	originalLength := len(input)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.overflow = true
		return originalLength, nil
	}
	if len(input) > remaining {
		b.overflow = true
		input = input[:remaining]
	}
	_, _ = b.buffer.Write(input)
	return originalLength, nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

type larkErrorEnvelope struct {
	OK    *bool `json:"ok"`
	Error struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"error"`
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

func classifyLarkCommandError(err error) (error, bool) {
	var commandErr *larkCommandError
	if !errors.As(err, &commandErr) {
		var adapterErr *AdapterError
		if errors.As(err, &adapterErr) {
			return err, true
		}
		return NewUnknownExternalResultError(
			"lark_result_unknown",
			"Feishu CLI result is unknown",
		), false
	}

	envelope, ok := decodeLarkErrorEnvelope(commandErr.stderr)
	if !ok {
		envelope, ok = decodeLarkErrorEnvelope(commandErr.stdout)
	}
	if !ok {
		if errors.Is(commandErr.cause, exec.ErrNotFound) {
			return NewAdapterError(
				"lark_cli_unavailable",
				"Feishu CLI is unavailable",
				true,
			), true
		}
		return NewUnknownExternalResultError(
			"lark_result_unknown",
			"Feishu CLI result is unknown",
		), false
	}

	errorType := strings.ToLower(strings.TrimSpace(envelope.Error.Type))
	code := rawJSONScalar(envelope.Error.Code)
	if code == "" {
		code = rawJSONScalar(envelope.Code)
	}
	message := strings.ToLower(strings.TrimSpace(envelope.Error.Message + " " + envelope.Message))
	switch {
	case strings.Contains(errorType, "auth"),
		strings.Contains(errorType, "login"),
		strings.Contains(errorType, "token_expired"),
		strings.Contains(message, "login"),
		strings.Contains(message, "token expired"):
		return NewAdapterError(
			"lark_auth_expired",
			"Feishu user login is required or expired",
			false,
		), true
	case strings.Contains(errorType, "permission"),
		strings.Contains(errorType, "forbidden"),
		strings.Contains(message, "permission"),
		code == "2091005":
		return NewAdapterError(
			"lark_permission_denied",
			"Feishu user permission is insufficient",
			false,
		), true
	case strings.Contains(errorType, "rate"),
		strings.Contains(message, "rate limit"),
		strings.Contains(message, "frequency"),
		code == "99991400":
		return NewAdapterError(
			"lark_rate_limited",
			"Feishu request rate is limited",
			true,
		), true
	case strings.Contains(errorType, "processing"),
		strings.Contains(errorType, "not_ready"),
		strings.Contains(message, "still processing"),
		strings.Contains(message, "not ready"):
		return errLarkMinutesPending, true
	case strings.Contains(errorType, "invalid"),
		strings.Contains(errorType, "unsupported"),
		strings.Contains(message, "unsupported"):
		return NewAdapterError(
			"lark_request_rejected",
			"Feishu rejected the media request",
			false,
		), true
	default:
		return NewUnknownExternalResultError(
			"lark_result_unknown",
			"Feishu CLI result is unknown",
		), false
	}
}

func decodeLarkErrorEnvelope(content []byte) (larkErrorEnvelope, bool) {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || len(content) > maxLarkCLIOutputBytes {
		return larkErrorEnvelope{}, false
	}
	var envelope larkErrorEnvelope
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&envelope); err != nil {
		return larkErrorEnvelope{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return larkErrorEnvelope{}, false
	}
	if envelope.Error.Type == "" &&
		len(envelope.Error.Code) == 0 &&
		len(envelope.Code) == 0 &&
		envelope.Message == "" &&
		envelope.Error.Message == "" {
		return larkErrorEnvelope{}, false
	}
	return envelope, true
}

func rawJSONScalar(value json.RawMessage) string {
	if len(value) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		return number.String()
	}
	var integer int64
	if json.Unmarshal(value, &integer) == nil {
		return strconv.FormatInt(integer, 10)
	}
	return ""
}

func strictJSONDecode(content []byte, destination any) error {
	if len(content) == 0 || len(content) > maxLarkCLIOutputBytes {
		return fmt.Errorf("invalid JSON output size")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON output")
	}
	return nil
}

func filepathIsCanonicalDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(resolved) != path {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
