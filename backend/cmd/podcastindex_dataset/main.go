package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/podcastindex/upgrade"
)

var errNoGo = errors.New("PodcastIndex dataset result is NO-GO")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		if errors.Is(err, errNoGo) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("subcommand is required")
	}
	switch args[0] {
	case "download":
		return runDownload(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "export-samples":
		return runExportSamples(args[1:])
	case "compare-samples":
		return runCompareSamples(args[1:])
	case "cutover":
		return runCutover(args[1:])
	case "rollback":
		return runRollback(args[1:])
	case "self-test":
		return upgrade.SelfTestCutoverAndRollback()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runDownload(args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	url := flags.String("url", upgrade.DefaultDownloadURL, "official PodcastIndex download URL")
	stagingDir := flags.String("staging-dir", "", "independent staging directory")
	liveDB := flags.String("live-db", "", "live database path used only to reject unsafe staging placement")
	manifestPath := flags.String("manifest", "", "manifest output path")
	proxyURL := flags.String("proxy", "", "explicit HTTP(S) proxy for this run; default is direct")
	timeout := flags.Duration("timeout", 30*time.Second, "HTTP timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *stagingDir == "" || *manifestPath == "" || *liveDB == "" {
		return fmt.Errorf("--staging-dir, --live-db and --manifest are required")
	}

	manifest := upgrade.NewManifest("download")
	manifest.Source.URL = *url
	manifest.Source.Transport = "direct"
	if *proxyURL != "" {
		endpoint, err := upgrade.ProxyEndpoint(*proxyURL)
		if err != nil {
			return err
		}
		manifest.Source.Transport = "explicit_http_proxy"
		manifest.Source.ProxyEndpoint = endpoint
	}
	client, err := upgrade.NewHTTPClient(*timeout, *proxyURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := upgrade.Download(ctx, upgrade.DownloadOptions{
		URL:        *url,
		StagingDir: *stagingDir,
		LiveDBPath: *liveDB,
		Client:     client,
	})
	if result != nil {
		manifest.Source.Before = &result.Before
		manifest.Source.After = result.After
		manifest.Source.SHA256 = result.SHA256
		manifest.Source.SizeBytes = result.SizeBytes
		manifest.Disk = result.Disk
		if result.Archive != nil {
			manifest.Archive = *result.Archive
		}
	}
	if err != nil {
		upgrade.AddBlocker(&manifest, "下载或 HTTP 指纹校验失败: "+err.Error())
		if writeErr := upgrade.WriteManifestAtomic(*manifestPath, manifest); writeErr != nil {
			return fmt.Errorf("%v; write manifest: %w", err, writeErr)
		}
		return err
	}
	downloadedAt := time.Now().UTC()
	manifest.Source.DownloadedAt = &downloadedAt
	upgrade.AddBlocker(&manifest, "尚未完成候选 SQLite Schema、查询和 146 样本验收")
	if err := upgrade.WriteManifestAtomic(*manifestPath, manifest); err != nil {
		return err
	}
	fmt.Printf("downloaded archive=%s sha256=%s\n", result.ArchivePath, result.SHA256)
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	stagingDir := flags.String("staging-dir", "", "independent staging directory")
	archivePath := flags.String("archive", "", "validated .tgz path; extracted when --db is absent")
	databasePath := flags.String("db", "", "candidate SQLite path inside staging")
	liveDB := flags.String("live-db", "", "current production database path for separation and baseline comparison")
	manifestPath := flags.String("manifest", "", "manifest output path")
	downloadManifestPath := flags.String("download-manifest", "", "download manifest to carry source fingerprint into this manifest")
	viewSQLPath := flags.String("view-sql", "scripts/create_unique_podcasts_view.sql", "project v_unique_podcasts SQL")
	baselineDB := flags.String("baseline-db", "", "read-only old database for metrics comparison")
	baselineMetricsJSON := flags.String("baseline-metrics-json", "", "JSON-encoded old database metrics for a remote read-only baseline")
	samplesPath := flags.String("samples", "", "JSON file containing the 146 failed samples")
	checkAccessibility := flags.Bool("check-accessibility", true, "directly check matched candidate feed URLs")
	accessibilityTimeout := flags.Duration("accessibility-timeout", 10*time.Second, "candidate URL check timeout")
	proxyURL := flags.String("proxy", "", "explicit HTTP(S) proxy for candidate URL checks; default is direct")
	maxQualityChange := flags.Float64("max-quality-change", 0.20, "maximum unexplained dataset quality change before No-Go")
	reviewed := flags.Bool("reviewed", false, "operator confirms the comparison and Human Review Queue evidence was reviewed")
	fullIntegrity := flags.Bool("full-integrity", false, "run PRAGMA integrity_check instead of quick_check")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *stagingDir == "" || *manifestPath == "" || *liveDB == "" {
		return fmt.Errorf("--staging-dir, --live-db and --manifest are required")
	}
	if *liveDB != "" {
		if err := upgrade.EnsureStagingSeparate(*stagingDir, *liveDB); err != nil {
			return err
		}
	}
	var accessibilityClient *http.Client
	if *proxyURL != "" {
		client, clientErr := upgrade.NewHTTPClient(*accessibilityTimeout, *proxyURL)
		if clientErr != nil {
			return clientErr
		}
		accessibilityClient = client
	}

	manifest := upgrade.NewManifest("validate")
	if *downloadManifestPath != "" {
		downloadManifest, err := upgrade.ReadManifest(*downloadManifestPath)
		if err != nil {
			return err
		}
		manifest.Source = downloadManifest.Source
		manifest.Archive = downloadManifest.Archive
		manifest.Disk = downloadManifest.Disk
	}

	if *archivePath == "" && manifest.Archive.ArchivePath != "" {
		*archivePath = manifest.Archive.ArchivePath
	}
	if *archivePath != "" {
		if !pathInside(*stagingDir, *archivePath) {
			return fmt.Errorf("archive must be inside staging directory")
		}
		inspection, err := upgrade.ValidateArchive(*archivePath)
		manifest.Archive = inspection
		if err != nil {
			upgrade.AddBlocker(&manifest, "压缩包安全校验失败: "+err.Error())
			return writeNoGo(*manifestPath, manifest, err)
		}
		archiveSHA256, archiveSize, err := upgrade.VerifyFileIdentity(*archivePath, manifest.Source.SHA256, manifest.Source.SizeBytes)
		if err != nil {
			upgrade.AddBlocker(&manifest, "压缩包本地身份与下载 manifest 不一致: "+err.Error())
			return writeNoGo(*manifestPath, manifest, err)
		}
		if manifest.Source.SHA256 == "" {
			manifest.Source.SHA256 = archiveSHA256
		}
		if manifest.Source.SizeBytes == 0 {
			manifest.Source.SizeBytes = archiveSize
		}
		if *databasePath == "" {
			extractionDir := filepath.Join(*stagingDir, "candidate")
			*databasePath, err = upgrade.ExtractArchive(*archivePath, extractionDir)
			if err != nil {
				upgrade.AddBlocker(&manifest, "SQLite 解压失败: "+err.Error())
				return writeNoGo(*manifestPath, manifest, err)
			}
		}
	}
	if *databasePath == "" {
		return fmt.Errorf("--db is required when --archive is absent")
	}
	if !pathInside(*stagingDir, *databasePath) {
		return fmt.Errorf("candidate database must be inside staging directory")
	}
	viewSQL, err := readViewSQL(*viewSQLPath)
	if err != nil {
		return err
	}
	validation, err := upgrade.ValidateCandidate(*databasePath, viewSQL, *fullIntegrity)
	manifest.Candidate = validation
	if err != nil {
		upgrade.AddBlocker(&manifest, "候选 SQLite 验收失败: "+err.Error())
	}

	if *baselineDB != "" && *baselineMetricsJSON != "" {
		return fmt.Errorf("use only one of --baseline-db and --baseline-metrics-json")
	}
	if *baselineDB != "" || *baselineMetricsJSON != "" {
		var baseline upgrade.DatabaseMetrics
		var metricsErr error
		if *baselineDB != "" {
			baseline, metricsErr = upgrade.ReadDatabaseMetrics(*baselineDB)
		} else {
			metricsErr = json.Unmarshal([]byte(*baselineMetricsJSON), &baseline)
			if metricsErr != nil {
				metricsErr = fmt.Errorf("decode baseline metrics JSON: %w", metricsErr)
			}
		}
		manifest.Baseline = &baseline
		if metricsErr != nil {
			upgrade.AddBlocker(&manifest, "旧库基线读取失败: "+metricsErr.Error())
		} else if validation.Passed {
			quality := upgrade.CompareDatabaseMetrics(baseline, validation.Metrics, *maxQualityChange)
			manifest.Quality = &quality
			for _, reason := range quality.Reasons {
				upgrade.AddBlocker(&manifest, "新旧数据质量对比未通过: "+reason)
			}
		}
	} else {
		upgrade.AddBlocker(&manifest, "缺少旧库只读基线，无法完成新旧数据量和质量对比")
	}

	if manifest.Archive.ExtractedBytes == 0 {
		upgrade.AddBlocker(&manifest, "缺少本次真实压缩包大小和解压大小证据")
	} else if manifest.Source.SizeBytes <= 0 || manifest.Source.SHA256 == "" {
		upgrade.AddBlocker(&manifest, "缺少本次压缩包 Content-Length 或 SHA-256 证据")
	} else {
		disk, diskErr := upgrade.EvaluateDiskGate(nil, *stagingDir, manifest.Source.SizeBytes, manifest.Archive.ExtractedBytes)
		manifest.Disk = disk
		if diskErr != nil {
			upgrade.AddBlocker(&manifest, "磁盘门槛不满足: "+diskErr.Error())
		}
	}

	if *samplesPath == "" {
		upgrade.AddBlocker(&manifest, "缺少 146 个失败节目样本")
	} else {
		samples, samplesErr := upgrade.ReadSamples(*samplesPath)
		if samplesErr != nil {
			upgrade.AddBlocker(&manifest, "失败样本读取失败: "+samplesErr.Error())
		} else if validation.Passed {
			comparison, compareErr := upgrade.CompareFailedSamples(context.Background(), *databasePath, samples, upgrade.CompareOptions{
				CheckAccessibility:   *checkAccessibility,
				AccessibilityClient:  accessibilityClient,
				AccessibilityTimeout: *accessibilityTimeout,
			})
			manifest.Comparison = &comparison
			if compareErr != nil {
				upgrade.AddBlocker(&manifest, "146 个失败样本对比失败: "+compareErr.Error())
			} else {
				if len(samples) != 146 {
					upgrade.AddBlocker(&manifest, fmt.Sprintf("失败样本数量为 %d，不是要求的 146", len(samples)))
				}
			}
		}
	}
	if manifest.Source.Before == nil || manifest.Source.After == nil {
		upgrade.AddBlocker(&manifest, "缺少下载前后完整 HTTP 对象指纹；本地压缩包来源不能替代 HEAD/GET 证据")
	} else if err := upgrade.CompareFingerprints(*manifest.Source.Before, *manifest.Source.After); err != nil {
		upgrade.AddBlocker(&manifest, "下载前后 HTTP 对象指纹不一致: "+err.Error())
	}

	if err := upgrade.SelfTestCutoverAndRollback(); err != nil {
		upgrade.AddBlocker(&manifest, "原子切换/回滚自测失败: "+err.Error())
	} else {
		manifest.Cutover.RollbackTested = true
	}
	if !*reviewed {
		upgrade.AddBlocker(&manifest, "尚未确认新旧质量差异和 Human Review Queue 结果已人工复核")
	}
	manifest.Decision.CheckedAt = time.Now().UTC()
	manifest.Decision.Go = len(manifest.Blockers) == 0
	if err := upgrade.WriteManifestAtomic(*manifestPath, manifest); err != nil {
		return err
	}
	fmt.Printf("validation=%s manifest=%s\n", decisionLabel(manifest.Decision.Go), *manifestPath)
	if !manifest.Decision.Go {
		return errNoGo
	}
	return nil
}

func runExportSamples(args []string) error {
	flags := flag.NewFlagSet("export-samples", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	primaryDB := flags.String("primary-db", "", "read-only MagicPodcast primary database")
	output := flags.String("output", "", "sample JSON output path")
	prefix := flags.String("feed-prefix", "https://feed.xyzfm.space/", "failed feed URL prefix")
	since := flags.String("since", upgrade.DefaultFailureSince, "failure sample start date")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *primaryDB == "" || *output == "" {
		return fmt.Errorf("--primary-db and --output are required")
	}
	samples, err := upgrade.ExportFailedSamplesSince(*primaryDB, *prefix, *since)
	if err != nil {
		return err
	}
	if err := upgrade.WriteSamples(*output, upgrade.NormalizeSamples(samples)); err != nil {
		return err
	}
	fmt.Printf("exported_samples=%d output=%s\n", len(samples), *output)
	return nil
}

func runCompareSamples(args []string) error {
	flags := flag.NewFlagSet("compare-samples", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	candidateDB := flags.String("candidate-db", "", "read-only PodcastIndex SQLite database")
	samplesPath := flags.String("samples", "", "failed sample JSON")
	output := flags.String("output", "", "optional JSON comparison output")
	checkAccessibility := flags.Bool("check-accessibility", true, "directly check matched candidate feed URLs")
	accessibilityTimeout := flags.Duration("accessibility-timeout", 10*time.Second, "candidate URL check timeout")
	proxyURL := flags.String("proxy", "", "explicit HTTP(S) proxy for candidate URL checks; default is direct")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *candidateDB == "" || *samplesPath == "" {
		return fmt.Errorf("--candidate-db and --samples are required")
	}
	samples, err := upgrade.ReadSamples(*samplesPath)
	if err != nil {
		return err
	}
	var accessibilityClient *http.Client
	if *proxyURL != "" {
		accessibilityClient, err = upgrade.NewHTTPClient(*accessibilityTimeout, *proxyURL)
		if err != nil {
			return err
		}
	}
	comparison, err := upgrade.CompareFailedSamples(context.Background(), *candidateDB, samples, upgrade.CompareOptions{
		CheckAccessibility:   *checkAccessibility,
		AccessibilityClient:  accessibilityClient,
		AccessibilityTimeout: *accessibilityTimeout,
	})
	if err != nil {
		return err
	}
	if *output != "" {
		if err := upgrade.WriteJSONAtomic(*output, comparison, 0o600); err != nil {
			return err
		}
	}
	fmt.Printf("samples=%d matched=%d identity_confirmed=%d title_only=%d accessible_identity_confirmed=%d output=%s\n",
		comparison.ActualSamples, comparison.Matched, comparison.IdentityConfirmed, comparison.TitleOnly,
		comparison.AccessibleIdentityConfirmed, *output)
	return nil
}

func runCutover(args []string) error {
	flags := flag.NewFlagSet("cutover", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	candidate := flags.String("candidate", "", "validated candidate database")
	live := flags.String("live-db", "", "production PodcastIndex database")
	manifestPath := flags.String("manifest", "", "validated manifest")
	serviceStopped := flags.Bool("service-stopped", false, "operator confirms service and all readers are stopped")
	dryRun := flags.Bool("dry-run", false, "verify without renaming files")
	confirmation := flags.String("confirm", "", "exact cutover confirmation string")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *candidate == "" || *live == "" || *manifestPath == "" {
		return fmt.Errorf("--candidate, --live-db and --manifest are required")
	}
	manifest, err := upgrade.ReadManifest(*manifestPath)
	if err != nil {
		return err
	}
	record, err := upgrade.Cutover(*candidate, *live, manifest, *serviceStopped, *dryRun, *confirmation)
	manifest.Cutover = record
	if writeErr := upgrade.WriteManifestAtomic(*manifestPath, manifest); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return err
	}
	fmt.Printf("cutover=%s backup=%s\n", record.Status, record.BackupPath)
	return nil
}

func runRollback(args []string) error {
	flags := flag.NewFlagSet("rollback", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	live := flags.String("live-db", "", "production PodcastIndex database")
	backup := flags.String("backup-db", "", "timestamped read-only rollback copy")
	manifestPath := flags.String("manifest", "", "manifest used to verify rollback copy")
	serviceStopped := flags.Bool("service-stopped", false, "operator confirms service and all readers are stopped")
	dryRun := flags.Bool("dry-run", false, "verify without renaming files")
	confirmation := flags.String("confirm", "", "exact rollback confirmation string")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *live == "" || *backup == "" || *manifestPath == "" {
		return fmt.Errorf("--live-db, --backup-db and --manifest are required")
	}
	manifest, err := upgrade.ReadManifest(*manifestPath)
	if err != nil {
		return err
	}
	record, err := upgrade.Rollback(*live, *backup, manifest, *serviceStopped, *dryRun, *confirmation)
	previousBackupPath := manifest.Cutover.BackupPath
	previousBackupSHA := manifest.Cutover.BackupSHA256
	record.BackupPath = previousBackupPath
	record.BackupSHA256 = previousBackupSHA
	record.RollbackTested = err == nil
	record.ProductionVerified = err == nil && !*dryRun
	manifest.Cutover = record
	if writeErr := upgrade.WriteManifestAtomic(*manifestPath, manifest); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return err
	}
	fmt.Printf("rollback=%s failed_candidate=%s\n", record.Status, record.FailedPath)
	return nil
}

func writeNoGo(path string, manifest upgrade.Manifest, cause error) error {
	if writeErr := upgrade.WriteManifestAtomic(path, manifest); writeErr != nil {
		return fmt.Errorf("%v; write manifest: %w", cause, writeErr)
	}
	return errNoGo
}

func readViewSQL(path string) (string, error) {
	paths := []string{path}
	if path == "scripts/create_unique_podcasts_view.sql" {
		paths = append(paths, "../scripts/create_unique_podcasts_view.sql")
	}
	for _, candidate := range paths {
		contents, err := os.ReadFile(candidate)
		if err == nil {
			return string(contents), nil
		}
	}
	return "", fmt.Errorf("read v_unique_podcasts SQL %s: file not found", path)
}

func pathInside(parent, child string) bool {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(parentAbs), filepath.Clean(childAbs))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func decisionLabel(goDecision bool) string {
	if goDecision {
		return "GO"
	}
	return "NO-GO"
}

func printUsage() {
	fmt.Print(`PodcastIndex dataset safety workflow

	  download       download a fixed official object into independent staging (direct by default; optional explicit proxy)
  validate       validate archive/SQLite/schema/view/queries/146-sample evidence
  export-samples export the known failed-program set from the primary DB read-only
  compare-samples compare the failed sample set against a read-only candidate/baseline DB
  cutover        same-filesystem atomic rename after explicit service stop
  rollback       restore a timestamped rollback copy after explicit service stop
  self-test      exercise atomic cutover and rollback in a temporary directory

The workflow never changes the MagicPodcast primary database or selects a Feed.
`)
}
