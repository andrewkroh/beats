package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/elastic/beats/winlogbeat/tests/benchmark/testing/integ"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

const (
	winlogbeatConfig = `# Generated
winlogbeat.event_logs:
- name: {{.EventLogName}}

output.file:
  path: {{.OutputPath}}/out
  filename: winlogbeat.out.json
  rotate_every_kb: 102400

logging.level: info
logging.files:
  path: {{.OutputPath}}/logs
  name: winlogbeat.log
`

	metricbeatConfig = `# Generated
metricbeat.modules:
- module: system
  metricsets: [cpu, diskio, filesystem, fsstat, network]
- module: system
  metricsets: [core, memory, process]
  processes: ['winlogbeat\.exe']
  period: 500ms

output.elasticsearch.hosts: ['{{.ElasticsearchURL}}']

logging.level: info
logging.files:
  path: {{.OutputPath}}/logs
  name: metricbeat.log
`
)

// Flags
var (
	testTimeout      = flag.Duration("timeout", 5*time.Minute, "Test timeout")
	elasticsearchURL = flag.String("es-url", "", "Elasticsearch URL where metrics will be sent")
)

var log = logrus.WithField("package", "benchmark")

// Templates
var (
	winlogbeatTemplate = template.Must(template.New("winlogbeat").Parse(winlogbeatConfig))
	metricbeatTemplate = template.Must(template.New("metricbeat").Parse(metricbeatConfig))
)

func extractGithubArchive(outputDir, user, project, commit string) (string, error) {
	const ghArchiveBaseURL = "https://github.com/%v/%v/archive/%v.zip"

	zipURL := fmt.Sprintf(ghArchiveBaseURL, user, project, commit)
	downloadFile, err := integ.DownloadFile(zipURL, outputDir)
	if err != nil {
		return "", err
	}
	defer os.Remove(downloadFile)

	err = integ.Extract(downloadFile, outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to extract github archive: %v", err)
	}

	return filepath.Join(outputDir, project+"-"+commit), nil
}

type WinlogbeatMetrics struct {
	PublishedEvents map[string]int64 `json:"published_events"`
}

func queryMetrics(url string) (*WinlogbeatMetrics, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	metrics := &WinlogbeatMetrics{}
	if err = json.Unmarshal(content, metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}

func waitForMetric(b *integ.Beat) error {
	u := fmt.Sprintf("http://%s/debug/vars", b.HTTPProfAddress)
	metrics, err := queryMetrics(u)
	if err != nil {
		return err
	}

	if metrics.PublishedEvents["Winlogbeat"] < 10000 {
		return fmt.Errorf("Winlogbeat total < 10000")
	}

	// Condition met.
	log.WithField("published_events", metrics.PublishedEvents).Info("metric condition met")
	return nil
}

func main() {
	flag.Parse()

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{})

	err := runBenchmark()
	if err != nil {
		log.WithError(err).Error("benchmark failed")
		os.Exit(1)
	}

	log.Info("success")
}

func runBenchmark() error {
	// Create temp directory.
	tempDir, err := ioutil.TempDir("", "wlb-benchmark")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}
	log.WithField("tempDir", tempDir).Info("created temp dir")

	// Setup Golang.
	goroot, err := integ.SetupGolang(tempDir)
	if err != nil {
		return errors.Wrap(err, "failed to setup golang")
	}
	log.WithField("GOROOT", tempDir).Info("golang setup complete")

	// Download Beats from Github.
	const user, project, commit = "elastic", "beats", "master"
	archiveDir, err := extractGithubArchive(tempDir, user, project, commit)
	if err != nil {
		return errors.Wrap(err, "failed to extract github archive")
	}
	log.WithField("archiveDir", archiveDir).Debug("extracted github archive")

	// Setup gopath structure.
	gopath := filepath.Join(tempDir, "gopath")
	elasticPath := filepath.Join(gopath, "src", "github.com", user)
	err = os.MkdirAll(elasticPath, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to mkdir")
	}

	// Move Beats into proper gopath location.
	err = os.Rename(archiveDir, filepath.Join(elasticPath, project))
	if err != nil {
		return errors.Wrap(err, "failed to move project dir onto gopath")
	}

	// Build Winlogbeat.
	winlogbeatPath := filepath.Join(tempDir, "winlogbeat"+integ.ExecutableExtension)
	output, err := integ.RunGoCmd(goroot, gopath, filepath.Join(elasticPath, project, "winlogbeat"), "build", "-o", winlogbeatPath)
	if err != nil {
		return errors.Wrapf(err, "winlogbeat build failed: %v", output)
	}

	// Build Metricbeat.
	metricbeatPath := filepath.Join(tempDir, "metricbeat"+integ.ExecutableExtension)
	output, err = integ.RunGoCmd(goroot, gopath, filepath.Join(elasticPath, project, "metricbeat"), "build", "-o", metricbeatPath)
	if err != nil {
		return errors.Wrapf(err, "metricbeat build failed: %v", output)
	}
	integ.CopyFile(filepath.Join(elasticPath, project, "metricbeat", "metricbeat.template.json"), filepath.Join(tempDir, "metricbeat.template.json"))
	integ.CopyFile(filepath.Join(elasticPath, project, "metricbeat", "metricbeat.template-es2x.json"), filepath.Join(tempDir, "metricbeat.template-es2x.json"))

	// Write Metricbeat template.
	metricbeatConfigFile := filepath.Join(tempDir, "metricbeat.yml")
	err = integ.WriteTemplateToFile(metricbeatTemplate, map[string]interface{}{
		"OutputPath":       tempDir,
		"ElasticsearchURL": elasticsearchURL,
	}, metricbeatConfigFile)
	if err != nil {
		return errors.Wrap(err, "failed to write metricbeat template")
	}

	// Write Winlogbeat template.
	winlogbeatConfigFile := filepath.Join(tempDir, "winlogbeat.yml")
	err = integ.WriteTemplateToFile(winlogbeatTemplate, map[string]interface{}{
		"OutputPath":   tempDir,
		"EventLogName": "Winlogbeat",
	}, winlogbeatConfigFile)
	if err != nil {
		return errors.Wrap(err, "failed to write metricbeat template")
	}

	metricbeat := &integ.Beat{
		Dir:        tempDir,
		Path:       metricbeatPath,
		ConfigFile: metricbeatConfigFile,
	}
	metricbeatCmd, err := metricbeat.Run(0)
	if err != nil {
		return errors.Wrap(err, "failed to run metricbeat")
	}
	defer func() {
		metricbeatCmd.SendCtrlCSignal()
		metricbeatCmd.Wait()
		log.Println("metricbeat output: ", metricbeat.CombinedOutput().String())
	}()

	winlogbeat := &integ.Beat{
		Dir:             tempDir,
		Path:            winlogbeatPath,
		ConfigFile:      winlogbeatConfigFile,
		HTTPProfAddress: "localhost:8123",
	}
	winlogbeatCmd, err := winlogbeat.RunWithCondition(*testTimeout, waitForMetric)
	if err != nil {
		log.WithField("output", winlogbeat.CombinedOutput().String()).Debugf("winlogbeat done")
		return errors.Wrap(err, "failed to run winlogbeat")
	}
	defer winlogbeatCmd.Process.Kill()

	// TODO: Create new eventlog with data.
	// TODO: Set size of eventlog to make it bigger.
	// TODO: Write 100k event logs using NASA logs.

	// TODO: Start Metricbeat.
	// TODO: Start Winlogbeat.
	// TODO: Wait for 100k to be read.
	// TODO: Stop Metricbeat.

	return nil
}
