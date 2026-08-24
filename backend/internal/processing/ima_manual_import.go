package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"magicpodcast/internal/models"
)

const (
	IMAManualImportAdapterVersion = "ima-manual-import-v1"
	IMAManualImportPackageSchema  = "magicpodcast.ima.manual_import.package"
	IMAManualImportSchemaVersion  = "1.0.0"
	DeliveryModeManualImport      = "manual_import"
	imaMaxNestedURLDepth          = 4
	imaMaxURLDecodePasses         = 8
)

var (
	imaHTTPURLPattern           = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
	imaSchemeRelativeURLPattern = regexp.MustCompile(`(?i)(?:^|[\s=("''])//[^\s<>"']+`)
	imaRootRelativeMarkdownLink = regexp.MustCompile(`!?\[[^\]\r\n]*\]\(/[^\s<>"')]+(?:\s+["'][^"'\r\n]*["'])?\)`)
	imaFileURLPattern           = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])file:(?:/{1,3}|\\+)`)
	imaUnixPath                 = regexp.MustCompile(`(?:^|[^\pL\pN_<>/])/[^\s<>"']+`)
	imaWindowsPath              = regexp.MustCompile(`(?i)(?:^|[^[:alnum:]_])[a-z]:[\\/][^\s<>"']+`)
	imaUNCPath                  = regexp.MustCompile(`\\\\[^\\\s<>"']+\\[^\s<>"']+`)
	imaLegacyIPv4Host           = regexp.MustCompile(`(?i)^(?:0x[0-9a-f]+|[0-9]+)(?:\.(?:0x[0-9a-f]+|[0-9]+)){0,3}$`)
	imaCredential               = regexp.MustCompile(
		`(?i)(?:^|[^a-z0-9])["']?(?:file[_-]?token|minute[_-]?token|access[_-]?token|refresh[_-]?token|id[_-]?token|token|api[_-]?key|access[_-]?key|secret|credential(?:s)?|password|passwd|authorization|cookie|session(?:[_-]?id)?|jwt|signature|sig)["']?\s*[:=]\s*["']?[^\s"',;]+`,
	)
)

type IMAManualImportBridge struct {
	root string
}

type imaPackageMetadata struct {
	Schema       string             `json:"schema"`
	Version      string             `json:"version"`
	DeliveryMode string             `json:"delivery_mode"`
	Target       string             `json:"target"`
	Destination  string             `json:"destination"`
	PackageID    string             `json:"package_id"`
	Episode      imaEpisodeMetadata `json:"episode"`
	Artifact     imaArtifactTrace   `json:"artifact"`
	Sources      map[string]string  `json:"sources"`
}

type imaEpisodeMetadata struct {
	ID          uint    `json:"id"`
	Title       string  `json:"title"`
	Podcast     string  `json:"podcast"`
	PublishedAt *string `json:"published_at"`
	SourceURL   string  `json:"source_url"`
	ShowNotes   string  `json:"show_notes"`
}

type imaArtifactTrace struct {
	RunID              uint   `json:"run_id"`
	ArtifactSetID      uint   `json:"artifact_set_id"`
	PipelineVersion    string `json:"pipeline_version"`
	GeneratedAt        string `json:"generated_at"`
	ManifestSHA256     string `json:"manifest_sha256"`
	TranscriptSHA256   string `json:"transcript_sha256"`
	EpisodeNotesSHA256 string `json:"episode_notes_sha256"`
}

type imaPackageManifest struct {
	Schema            string            `json:"schema"`
	Version           string            `json:"version"`
	DeliveryMode      string            `json:"delivery_mode"`
	Target            string            `json:"target"`
	Destination       string            `json:"destination"`
	PackageID         string            `json:"package_id"`
	ChecksumAlgorithm string            `json:"checksum_algorithm"`
	Artifact          imaArtifactTrace  `json:"artifact"`
	Files             []imaManifestFile `json:"files"`
}

type imaManifestFile struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	Size      int    `json:"size"`
	SHA256    string `json:"sha256"`
}

func NewIMAManualImportBridge(root string) (*IMAManualImportBridge, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("ima manual import root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve ima manual import root: %w", err)
	}
	if filepath.Clean(absolute) == filepath.Clean(string(os.PathSeparator)) {
		return nil, fmt.Errorf("ima manual import root cannot be the filesystem root")
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create ima manual import root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect ima manual import root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("ima manual import root must be a real directory")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("protect ima manual import root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical ima manual import root: %w", err)
	}
	return &IMAManualImportBridge{root: filepath.Clean(canonical)}, nil
}

func (*IMAManualImportBridge) Target() string         { return "ima" }
func (*IMAManualImportBridge) AdapterVersion() string { return IMAManualImportAdapterVersion }
func (*IMAManualImportBridge) DeliveryMode() string   { return DeliveryModeManualImport }

func (b *IMAManualImportBridge) Deliver(
	ctx context.Context,
	request DeliveryRequest,
) (DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	if err := validateIMAManualImportRequest(request); err != nil {
		return DeliveryReceipt{}, err
	}
	files, err := renderIMAManualImportPackage(request)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	if err := b.publishPackage(ctx, request, files); err != nil {
		return DeliveryReceipt{}, err
	}
	packageManifestSHA256, err := b.packageManifestSHA256(request.DeliveryKey)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	return DeliveryReceipt{
		RemoteRef: "ima-manual-import:" + request.DeliveryKey + ":" + packageManifestSHA256,
		Status:    models.DeliveryStatusPending,
	}, nil
}

func (*IMAManualImportBridge) Cancel(context.Context, uint) error { return nil }

func validateIMAManualImportRequest(request DeliveryRequest) error {
	pkg := request.Package
	switch {
	case request.ArtifactSetID == 0:
		return invalidIMAPackage("published artifact set is required")
	case !isLowerSHA256(request.DeliveryKey):
		return invalidIMAPackage("delivery key must be a SHA-256 value")
	case !safeDestination(request.Destination):
		return invalidIMAPackage("destination is not a safe identity")
	case pkg.RunID == 0 || pkg.EpisodeID == 0:
		return invalidIMAPackage("run and episode identity are required")
	case !safeSingleLine(pkg.EpisodeTitle) || !safeSingleLine(pkg.PodcastTitle):
		return invalidIMAPackage("episode title and podcast are required")
	case strings.TrimSpace(pkg.PipelineVersion) == "":
		return invalidIMAPackage("pipeline version is required")
	case pkg.ArtifactGeneratedAt.IsZero():
		return invalidIMAPackage("artifact generation time is required")
	case !isLowerSHA256(pkg.ManifestSHA256) ||
		!isLowerSHA256(pkg.TranscriptSHA256) ||
		!isLowerSHA256(pkg.EpisodeNotesSHA256):
		return invalidIMAPackage("artifact checksums are incomplete")
	case strings.TrimSpace(pkg.Transcript) == "" ||
		strings.TrimSpace(pkg.EpisodeNotes) == "":
		return invalidIMAPackage("transcript and episode notes are required")
	}
	if digestString(pkg.Transcript) != pkg.TranscriptSHA256 ||
		digestString(pkg.EpisodeNotes) != pkg.EpisodeNotesSHA256 {
		return invalidIMAPackage("artifact content does not match its checksum")
	}
	if strings.TrimSpace(pkg.SourceURL) != "" {
		if err := validateSafeHTTPURL(pkg.SourceURL); err != nil {
			return invalidIMAPackage("episode source URL is unsafe")
		}
	}
	for _, field := range []string{
		pkg.EpisodeTitle,
		pkg.PodcastTitle,
		pkg.ShowNotes,
		pkg.Transcript,
		pkg.EpisodeNotes,
	} {
		if err := validateIMAPackageText(field); err != nil {
			return err
		}
	}
	for key, value := range pkg.Sources {
		if !safeSingleLine(key) || sensitiveSourceKey(key) {
			return invalidIMAPackage("source trace contains a sensitive key")
		}
		if err := validateIMASourceTrace(key, value); err != nil {
			return err
		}
	}
	return nil
}

func renderIMAManualImportPackage(
	request DeliveryRequest,
) (map[string][]byte, error) {
	trace := imaTraceForRequest(request)
	metadata := imaPackageMetadata{
		Schema:       IMAManualImportPackageSchema + ".metadata",
		Version:      IMAManualImportSchemaVersion,
		DeliveryMode: DeliveryModeManualImport,
		Target:       "ima",
		Destination:  request.Destination,
		PackageID:    request.DeliveryKey,
		Episode: imaEpisodeMetadata{
			ID:          request.Package.EpisodeID,
			Title:       strings.TrimSpace(request.Package.EpisodeTitle),
			Podcast:     strings.TrimSpace(request.Package.PodcastTitle),
			PublishedAt: optionalStableTime(request.Package.PublishedAt),
			SourceURL:   strings.TrimSpace(request.Package.SourceURL),
			ShowNotes:   strings.TrimSpace(request.Package.ShowNotes),
		},
		Artifact: trace,
		Sources:  cloneStringMap(request.Package.Sources),
	}
	metadataBytes, err := marshalStableJSON(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode ima metadata: %w", err)
	}
	files := map[string][]byte{
		"knowledge.md":  []byte(renderIMAKnowledge(request, trace)),
		"metadata.json": metadataBytes,
		"IMPORT.md":     []byte(renderIMAImportInstructions(request.DeliveryKey)),
	}
	manifestFiles := make([]imaManifestFile, 0, len(files))
	for _, name := range []string{"knowledge.md", "metadata.json", "IMPORT.md"} {
		content := files[name]
		mediaType := "text/markdown; charset=utf-8"
		if filepath.Ext(name) == ".json" {
			mediaType = "application/json"
		}
		manifestFiles = append(manifestFiles, imaManifestFile{
			Path:      name,
			MediaType: mediaType,
			Size:      len(content),
			SHA256:    digestBytes(content),
		})
	}
	manifest := imaPackageManifest{
		Schema:            IMAManualImportPackageSchema + ".manifest",
		Version:           IMAManualImportSchemaVersion,
		DeliveryMode:      DeliveryModeManualImport,
		Target:            "ima",
		Destination:       request.Destination,
		PackageID:         request.DeliveryKey,
		ChecksumAlgorithm: "SHA-256",
		Artifact:          trace,
		Files:             manifestFiles,
	}
	files["manifest.json"], err = marshalStableJSON(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode ima package manifest: %w", err)
	}
	return files, nil
}

func renderIMAKnowledge(request DeliveryRequest, trace imaArtifactTrace) string {
	pkg := request.Package
	showNotes := strings.TrimSpace(pkg.ShowNotes)
	if showNotes == "" {
		showNotes = "_源数据未提供 Show Notes。_"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n", strings.TrimSpace(pkg.EpisodeTitle))
	builder.WriteString("## 单集信息\n\n")
	fmt.Fprintf(&builder, "- 节目：%s\n", strings.TrimSpace(pkg.PodcastTitle))
	publishedDate := "不可用"
	if !pkg.PublishedAt.IsZero() {
		publishedDate = pkg.PublishedAt.UTC().Format("2006-01-02")
	}
	fmt.Fprintf(&builder, "- 发布日期：%s\n", publishedDate)
	if sourceURL := strings.TrimSpace(pkg.SourceURL); sourceURL != "" {
		fmt.Fprintf(&builder, "- 来源：[%s](%s)\n", sourceURL, sourceURL)
	} else {
		builder.WriteString("- 来源：未提供公开链接\n")
	}
	fmt.Fprintf(&builder, "- 包 Schema：`%s/%s`\n", IMAManualImportPackageSchema, IMAManualImportSchemaVersion)
	fmt.Fprintf(&builder, "- 产物集：`%d`\n", request.ArtifactSetID)
	fmt.Fprintf(&builder, "- 产物 Manifest SHA-256：`%s`\n\n", trace.ManifestSHA256)
	builder.WriteString("## Show Notes\n\n")
	builder.WriteString(showNotes)
	builder.WriteString("\n\n## 单集纪要\n\n")
	builder.WriteString(strings.TrimSpace(pkg.EpisodeNotes))
	builder.WriteString("\n\n## 规范逐字稿\n\n")
	builder.WriteString(strings.TrimSpace(pkg.Transcript))
	if len(pkg.Sources) > 0 {
		builder.WriteString("\n\n## 来源追溯\n\n")
		keys := make([]string, 0, len(pkg.Sources))
		for key := range pkg.Sources {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&builder, "- `%s`：[%s](%s)\n", key, pkg.Sources[key], pkg.Sources[key])
		}
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

func renderIMAImportInstructions(packageID string) string {
	return fmt.Sprintf(`# ima 人工导入

包 ID：`+"`%s`"+`

1. 在 ima 官方产品中打开获批的测试知识库。
2. 先选择本包的 `+"`knowledge.md`"+` 进行人工上传；其余 JSON/说明文件仅用于本地校验与追溯。
3. 导入后人工核对标题、Show Notes、单集纪要、逐字稿与来源链接。
4. 重复导入、覆盖、批量文件和 JSON 支持情况尚未验证，不要推断产品行为。

当前状态仅为“包已生成 / 待人工导入”，不代表 ima 已接收、索引或交付成功。不要在验收记录中保存账号、Cookie、Token、二维码或会话。
`, packageID)
}

func (b *IMAManualImportBridge) publishPackage(
	ctx context.Context,
	request DeliveryRequest,
	files map[string][]byte,
) error {
	if err := ensureRealDirectory(b.root, 0o700); err != nil {
		return err
	}
	packagesRoot := filepath.Join(b.root, "packages")
	if err := ensureRealDirectory(packagesRoot, 0o700); err != nil {
		return err
	}
	finalPath := filepath.Join(packagesRoot, request.DeliveryKey)
	if _, err := os.Lstat(finalPath); err == nil {
		return verifyExistingIMAPackage(finalPath, request)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect ima package destination: %w", err)
	}

	stagingPath, err := os.MkdirTemp(packagesRoot, ".ima-staging-")
	if err != nil {
		return fmt.Errorf("create ima package staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingPath)
		}
	}()
	if err := os.Chmod(stagingPath, 0o700); err != nil {
		return fmt.Errorf("protect ima package staging directory: %w", err)
	}
	for _, name := range []string{"knowledge.md", "metadata.json", "manifest.json", "IMPORT.md"} {
		if err := writeExclusiveFile(filepath.Join(stagingPath, name), files[name]); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		if verifyErr := verifyExistingIMAPackage(finalPath, request); verifyErr == nil {
			return nil
		}
		return fmt.Errorf("publish ima package: %w", err)
	}
	published = true
	return verifyIMAPackage(finalPath, files)
}

func verifyExistingIMAPackage(root string, request DeliveryRequest) error {
	files, err := readStrictIMAPackage(root)
	if err != nil {
		return err
	}
	var manifest imaPackageManifest
	if err := unmarshalStrictJSON(files["manifest.json"], &manifest); err != nil {
		return invalidIMAPackage("published package manifest is invalid")
	}
	expectedTrace := imaTraceForRequest(request)
	if manifest.Schema != IMAManualImportPackageSchema+".manifest" ||
		manifest.Version != IMAManualImportSchemaVersion ||
		manifest.DeliveryMode != DeliveryModeManualImport ||
		manifest.Target != "ima" ||
		manifest.Destination != request.Destination ||
		manifest.PackageID != request.DeliveryKey ||
		manifest.ChecksumAlgorithm != "SHA-256" ||
		manifest.Artifact != expectedTrace {
		return invalidIMAPackage("published package identity does not match")
	}
	expectedPayloads := map[string]string{
		"knowledge.md":  "text/markdown; charset=utf-8",
		"metadata.json": "application/json",
		"IMPORT.md":     "text/markdown; charset=utf-8",
	}
	if len(manifest.Files) != len(expectedPayloads) {
		return invalidIMAPackage("published package checksum list is incomplete")
	}
	for _, file := range manifest.Files {
		mediaType, ok := expectedPayloads[file.Path]
		if !ok || file.MediaType != mediaType {
			return invalidIMAPackage("published package checksum path is invalid")
		}
		delete(expectedPayloads, file.Path)
		content := files[file.Path]
		if file.Size != len(content) || file.SHA256 != digestBytes(content) {
			return invalidIMAPackage("published package checksum does not match")
		}
	}
	if len(expectedPayloads) != 0 {
		return invalidIMAPackage("published package checksum list is incomplete")
	}

	var metadata imaPackageMetadata
	if err := unmarshalStrictJSON(files["metadata.json"], &metadata); err != nil {
		return invalidIMAPackage("published package metadata is invalid")
	}
	if metadata.Schema != IMAManualImportPackageSchema+".metadata" ||
		metadata.Version != IMAManualImportSchemaVersion ||
		metadata.DeliveryMode != DeliveryModeManualImport ||
		metadata.Target != "ima" ||
		metadata.Destination != request.Destination ||
		metadata.PackageID != request.DeliveryKey ||
		metadata.Artifact != expectedTrace ||
		metadata.Episode.ID != request.Package.EpisodeID ||
		!safeSingleLine(metadata.Episode.Title) ||
		!safeSingleLine(metadata.Episode.Podcast) {
		return invalidIMAPackage("published package metadata identity does not match")
	}
	if metadata.Episode.PublishedAt != nil {
		if _, err := time.Parse(time.RFC3339Nano, *metadata.Episode.PublishedAt); err != nil {
			return invalidIMAPackage("published package publication date is invalid")
		}
	}
	if err := validateIMAPackageText(metadata.Episode.Title); err != nil {
		return err
	}
	if err := validateIMAPackageText(metadata.Episode.Podcast); err != nil {
		return err
	}
	if strings.TrimSpace(metadata.Episode.SourceURL) != "" {
		if err := validateSafeHTTPURL(metadata.Episode.SourceURL); err != nil {
			return invalidIMAPackage("published package source URL is unsafe")
		}
	}
	if err := validateIMAPackageText(metadata.Episode.ShowNotes); err != nil {
		return err
	}
	for key, value := range metadata.Sources {
		if !safeSingleLine(key) || sensitiveSourceKey(key) {
			return invalidIMAPackage("published package source trace contains a sensitive key")
		}
		if err := validateSafeHTTPURL(value); err != nil {
			return invalidIMAPackage("published package source trace contains an unsafe URL")
		}
	}
	if err := validateIMAPackageText(string(files["knowledge.md"])); err != nil {
		return err
	}
	if string(files["IMPORT.md"]) != renderIMAImportInstructions(request.DeliveryKey) {
		return invalidIMAPackage("published package instructions do not match")
	}
	return nil
}

func verifyIMAPackage(root string, expected map[string][]byte) error {
	actual, err := readStrictIMAPackage(root)
	if err != nil {
		return err
	}
	for name, content := range expected {
		if digestBytes(actual[name]) != digestBytes(content) {
			return invalidIMAPackage("published package content does not match")
		}
	}
	return nil
}

func (b *IMAManualImportBridge) packageManifestSHA256(packageID string) (string, error) {
	manifestPath := filepath.Join(b.root, "packages", packageID, "manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return "", fmt.Errorf("inspect ima package manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != 0o600 {
		return "", invalidIMAPackage("published package manifest is unsafe")
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("read ima package manifest: %w", err)
	}
	return digestBytes(content), nil
}

func readStrictIMAPackage(root string) (map[string][]byte, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect ima package: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return nil, invalidIMAPackage("published package directory is unsafe")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list ima package: %w", err)
	}
	expectedNames := map[string]struct{}{
		"knowledge.md":  {},
		"metadata.json": {},
		"manifest.json": {},
		"IMPORT.md":     {},
	}
	if len(entries) != len(expectedNames) {
		return nil, invalidIMAPackage("published package contains unexpected files")
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if _, ok := expectedNames[entry.Name()]; !ok ||
			entry.Type()&os.ModeSymlink != 0 {
			return nil, invalidIMAPackage("published package contains an unsafe file")
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect ima package file: %w", err)
		}
		if !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0o600 {
			return nil, invalidIMAPackage("published package file permissions are unsafe")
		}
		content, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read ima package file: %w", err)
		}
		files[entry.Name()] = content
	}
	return files, nil
}

func ensureRealDirectory(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		if err := os.Mkdir(path, mode); err != nil {
			return fmt.Errorf("create ima package directory: %w", err)
		}
		info, err = os.Lstat(path)
	case err != nil:
		return fmt.Errorf("inspect ima package directory: %w", err)
	}
	if err != nil {
		return fmt.Errorf("inspect ima package directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return invalidIMAPackage("ima package path contains a symbolic link")
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("protect ima package directory: %w", err)
	}
	return nil
}

func writeExclusiveFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create ima package file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write ima package file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync ima package file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close ima package file: %w", err)
	}
	return nil
}

func marshalStableJSON(value any) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func unmarshalStrictJSON(content []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

func invalidIMAPackage(message string) error {
	return NewAdapterError("invalid_ima_manual_import_package", message, false)
}

func safeDestination(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || value == "." || value == ".." || filepath.IsAbs(value) ||
		strings.ContainsAny(value, `/\`) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func safeSingleLine(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\r\n\x00")
}

func sensitiveSourceKey(key string) bool {
	normalized := strings.ToLower(key)
	compact := strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' {
			return character
		}
		return -1
	}, normalized)
	for _, marker := range []string{
		"accesstoken", "apikey", "accesskey", "signature", "credential",
		"password", "sessionid", "authkey", "filetoken", "minutetoken",
		"minuteurl", "private note", "localpath",
	} {
		if strings.Contains(compact, strings.ReplaceAll(marker, " ", "")) {
			return true
		}
	}
	parts := strings.FieldsFunc(normalized, func(character rune) bool {
		return character < 'a' || character > 'z'
	})
	for _, part := range parts {
		switch part {
		case "token", "secret", "credential", "credentials", "password",
			"passwd", "cookie", "session", "authorization", "authentication",
			"auth", "apikey", "key", "signature", "sig", "jwt", "path":
			return true
		}
	}
	return strings.Contains(normalized, "file_token") ||
		strings.Contains(normalized, "minute_token") ||
		strings.Contains(normalized, "private_note") ||
		strings.Contains(normalized, "local_path")
}

func validateIMASourceTrace(key string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return invalidIMAPackage("source trace is incomplete")
	}
	if strings.HasPrefix(value, "sha256:") {
		if isLowerSHA256(strings.TrimPrefix(value, "sha256:")) {
			return nil
		}
		return invalidIMAPackage("source trace contains an invalid digest")
	}
	if validateSafeHTTPURL(value) == nil {
		return nil
	}
	if key != "transcription" {
		return invalidIMAPackage("source trace contains an unsafe value")
	}
	if len(value) > 100 {
		return invalidIMAPackage("source trace identity is too long")
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return invalidIMAPackage("source trace contains an unsafe value")
	}
	return nil
}

func validateIMAPackageText(value string) error {
	for _, raw := range imaHTTPURLPattern.FindAllString(value, -1) {
		candidate := strings.TrimRight(raw, ".,;:!?)]}")
		if err := validateSafeHTTPURL(candidate); err != nil {
			return invalidIMAPackage("package content contains an unsafe URL")
		}
	}
	textWithoutHTTPURLs := imaHTTPURLPattern.ReplaceAllString(value, "")
	textWithoutHTTPURLs = imaRootRelativeMarkdownLink.ReplaceAllString(
		textWithoutHTTPURLs,
		"",
	)
	if imaCredential.MatchString(value) {
		return invalidIMAPackage("package content contains credentials")
	}
	if imaFileURLPattern.MatchString(textWithoutHTTPURLs) ||
		strings.Contains(textWithoutHTTPURLs, "~/") ||
		imaUnixPath.MatchString(textWithoutHTTPURLs) ||
		imaWindowsPath.MatchString(textWithoutHTTPURLs) ||
		imaUNCPath.MatchString(textWithoutHTTPURLs) {
		return invalidIMAPackage("package content contains a local path")
	}
	return nil
}

func validateSafeHTTPURL(raw string) error {
	return validateSafeHTTPURLDepth(raw, 0)
}

func validateSafeHTTPURLDepth(raw string, depth int) error {
	if depth > imaMaxNestedURLDepth {
		return fmt.Errorf("URL nesting exceeds the safety limit")
	}
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return fmt.Errorf("URL must be valid")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("URL must be HTTP(S) without credentials")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" ||
		hostname == "localhost" ||
		strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".internal") ||
		strings.HasSuffix(hostname, ".home.arpa") {
		return fmt.Errorf("URL host must be public")
	}
	if address, parseErr := netip.ParseAddr(hostname); parseErr == nil {
		if !address.IsGlobalUnicast() ||
			address.IsPrivate() ||
			address.IsLoopback() ||
			address.IsLinkLocalUnicast() ||
			address.IsLinkLocalMulticast() ||
			address.IsUnspecified() {
			return fmt.Errorf("URL address must be public")
		}
	} else if imaLegacyIPv4Host.MatchString(hostname) {
		return fmt.Errorf("URL host uses a non-canonical numeric address")
	}
	for _, privateHost := range []string{
		"feishu.cn",
		"larkoffice.com",
		"larksuite.com",
	} {
		if hostname == privateHost || strings.HasSuffix(hostname, "."+privateHost) {
			return fmt.Errorf("URL host may expose a Feishu token")
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return fmt.Errorf("URL query must be valid")
	}
	for key, values := range query {
		if sensitiveURLKey(key) {
			return fmt.Errorf("URL query contains credentials")
		}
		for _, value := range values {
			if err := validateNestedHTTPURLs(value, depth+1); err != nil {
				return err
			}
		}
	}
	if parsed.Fragment != "" {
		if err := validateNestedHTTPURLs(parsed.Fragment, depth+1); err != nil {
			return err
		}
		fragment, err := url.ParseQuery(parsed.Fragment)
		if err != nil {
			return fmt.Errorf("URL fragment must be valid")
		}
		for key, values := range fragment {
			if sensitiveURLKey(key) {
				return fmt.Errorf("URL fragment contains credentials")
			}
			for _, value := range values {
				if err := validateNestedHTTPURLs(value, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateNestedHTTPURLs(value string, depth int) error {
	decodedValues := []string{value}
	for range imaMaxURLDecodePasses {
		decoded, err := url.QueryUnescape(decodedValues[len(decodedValues)-1])
		if err != nil {
			return fmt.Errorf("nested URL encoding must be valid")
		}
		if decoded == decodedValues[len(decodedValues)-1] {
			break
		}
		decodedValues = append(decodedValues, decoded)
	}
	lastDecoded := decodedValues[len(decodedValues)-1]
	if decoded, err := url.QueryUnescape(lastDecoded); err != nil {
		return fmt.Errorf("nested URL encoding must be valid")
	} else if decoded != lastDecoded {
		return fmt.Errorf("nested URL encoding exceeds the safety limit")
	}
	for _, decoded := range decodedValues {
		for _, raw := range imaHTTPURLPattern.FindAllString(decoded, -1) {
			candidate := strings.TrimRight(raw, ".,;:!?)]}")
			if err := validateSafeHTTPURLDepth(candidate, depth); err != nil {
				return fmt.Errorf("nested URL is unsafe: %w", err)
			}
		}
		for _, raw := range imaSchemeRelativeURLPattern.FindAllString(decoded, -1) {
			candidate := strings.TrimLeft(raw, " \t\r\n=(\"'")
			candidate = strings.TrimRight(candidate, ".,;:!?)]}")
			if err := validateSafeHTTPURLDepth("https:"+candidate, depth); err != nil {
				return fmt.Errorf("nested URL is unsafe: %w", err)
			}
		}
	}
	return nil
}

func sensitiveURLKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if strings.HasPrefix(normalized, "x-amz-") ||
		strings.HasPrefix(normalized, "x-goog-") {
		return true
	}
	compact := strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' {
			return character
		}
		return -1
	}, normalized)
	for _, marker := range []string{
		"accesstoken", "apikey", "accesskey", "signature", "credential",
		"password", "sessionid", "authkey",
	} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	parts := strings.FieldsFunc(normalized, func(character rune) bool {
		return character < 'a' || character > 'z'
	})
	for _, part := range parts {
		switch part {
		case "token", "key", "secret", "signature", "sig", "auth",
			"authorization", "credential", "password", "passwd", "cookie",
			"session", "jwt":
			return true
		}
	}
	return false
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func digestString(value string) string {
	return digestBytes([]byte(value))
}

func stableTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalStableTime(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := stableTime(value)
	return &formatted
}

func imaTraceForRequest(request DeliveryRequest) imaArtifactTrace {
	return imaArtifactTrace{
		RunID:              request.Package.RunID,
		ArtifactSetID:      request.ArtifactSetID,
		PipelineVersion:    strings.TrimSpace(request.Package.PipelineVersion),
		GeneratedAt:        stableTime(request.Package.ArtifactGeneratedAt),
		ManifestSHA256:     request.Package.ManifestSHA256,
		TranscriptSHA256:   request.Package.TranscriptSHA256,
		EpisodeNotesSHA256: request.Package.EpisodeNotesSHA256,
	}
}

var _ KnowledgeBridge = (*IMAManualImportBridge)(nil)
