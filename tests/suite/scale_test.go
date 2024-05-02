package suite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nginxinc/nginx-gateway-fabric/tests/framework"
)

var _ = Describe("Scale test", Ordered, Label("nfr", "scale"), func() {
	// One of the tests - scales upstream servers - requires a big cluster to provision 648 pods.
	// On GKE, you can use the following configuration:
	// - A Kubernetes cluster with 12 nodes on GKE
	// - Node: n2d-standard-16 (16 vCPU, 64GB memory)

	var (
		matchesManifests = []string{
			"scale/matches.yaml",
		}
		upstreamsManifests = []string{
			"scale/upstreams.yaml",
		}

		ns = &core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scale",
			},
		}

		scrapeInterval = 15 * time.Second
		queryRangeStep = 15 * time.Second

		resultsDir            string
		outFile               *os.File
		ngfPodName            string
		promInstance          framework.PrometheusInstance
		promPortForwardStopCh = make(chan struct{})
	)

	const (
		httpListenerCount   = 64
		httpsListenerCount  = 64
		httpRouteCount      = 1000
		upstreamServerCount = 648
	)

	BeforeAll(func() {
		// Scale tests need a dedicated NGF instance
		// Because they analyze the logs of NGF and NGINX, and they don't want to analyze the logs of other tests.
		teardown(releaseName)

		Expect(resourceManager.Apply([]client.Object{ns})).To(Succeed())

		var err error
		resultsDir, err = framework.CreateResultsDir("scale", version)
		Expect(err).ToNot(HaveOccurred())

		filename := filepath.Join(resultsDir, fmt.Sprintf("%s.md", version))
		outFile, err = framework.CreateResultsFile(filename)
		Expect(err).ToNot(HaveOccurred())
		Expect(framework.WriteSystemInfoToFile(outFile, clusterInfo, *plusEnabled)).To(Succeed())

		promCfg := framework.PrometheusConfig{
			ScrapeInterval: scrapeInterval,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		promInstance, err = framework.InstallPrometheus(ctx, resourceManager, promCfg)
		Expect(err).ToNot(HaveOccurred())

		k8sConfig := ctlr.GetConfigOrDie()

		if !clusterInfo.IsGKE {
			Expect(promInstance.PortForward(k8sConfig, promPortForwardStopCh)).To(Succeed())
		}
	})

	BeforeEach(func() {
		// Scale tests need a dedicated NGF instance per test.
		// Because they analyze the logs of NGF and NGINX, and they don't want to analyze the logs of other tests.
		cfg := getDefaultSetupCfg()
		labelFilter := GinkgoLabelFilter()
		cfg.nfr = isNFR(labelFilter)
		setup(cfg)

		podNames, err := framework.GetReadyNGFPodNames(k8sClient, ngfNamespace, releaseName, timeoutConfig.GetTimeout)
		Expect(err).ToNot(HaveOccurred())
		Expect(podNames).To(HaveLen(1))
		ngfPodName = podNames[0]
	})

	createResponseChecker := func(url, address string) func() error {
		return func() error {
			status, _, err := framework.Get(url, address, timeoutConfig.RequestTimeout)
			if err != nil {
				return fmt.Errorf("bad response: %w", err)
			}

			if status != 200 {
				return fmt.Errorf("unexpected status code: %d", status)
			}

			return nil
		}
	}

	createMetricExistChecker := func(query string, getTime func() time.Time, modifyTime func()) func() error {
		return func() error {
			queryWithTimestamp := fmt.Sprintf("%s @ %d", query, getTime().Unix())

			result, err := promInstance.Query(queryWithTimestamp)
			if err != nil {
				return fmt.Errorf("failed to query Prometheus: %w", err)
			}

			if result.String() == "" {
				modifyTime()
				return errors.New("empty result")
			}

			return nil
		}
	}

	createEndTimeFinder := func(query string, startTime time.Time, t *time.Time) func() error {
		return func() error {
			result, err := promInstance.QueryRange(query, v1.Range{
				Start: startTime,
				End:   *t,
				Step:  queryRangeStep,
			})
			if err != nil {
				return fmt.Errorf("failed to query Prometheus: %w", err)
			}

			if result.String() == "" {
				*t = time.Now()
				return errors.New("empty result")
			}

			return nil
		}
	}

	getFirstValueOfVector := func(query string) float64 {
		result, err := promInstance.Query(query)
		Expect(err).ToNot(HaveOccurred())

		val, err := framework.GetFirstValueOfPrometheusVector(result)
		Expect(err).ToNot(HaveOccurred())

		return val
	}

	getBuckets := func(query string) []bucket {
		result, err := promInstance.Query(query)
		Expect(err).ToNot(HaveOccurred())

		res, ok := result.(model.Vector)
		Expect(ok).To(BeTrue())

		buckets := make([]bucket, 0, len(res))

		for _, sample := range res {
			le := sample.Metric["le"]
			val := float64(sample.Value)
			bucket := bucket{
				Le:  string(le),
				Val: int(val),
			}
			buckets = append(buckets, bucket)
		}

		return buckets
	}

	checkLogErrors := func(containerName string, substrings []string, fileName string) int {
		logs, err := resourceManager.GetPodLogs(ngfNamespace, ngfPodName, &core.PodLogOptions{
			Container: containerName,
		})
		Expect(err).ToNot(HaveOccurred())

		logLines := strings.Split(logs, "\n")
		errors := 0

	outer:
		for _, line := range logLines {
			for _, substr := range substrings {
				if strings.Contains(line, substr) {
					errors++
					continue outer
				}
			}
		}

		// attach full logs
		if errors > 0 {
			f, err := os.Create(fileName)
			Expect(err).ToNot(HaveOccurred())
			defer f.Close()

			_, err = io.WriteString(f, logs)
			Expect(err).ToNot(HaveOccurred())
		}
		return errors
	}

	runTestWithMetricsAndLogs := func(testName, testResultsDir string, test func()) {
		var (
			metricExistTimeout = 2 * time.Minute
			metricExistPolling = 1 * time.Second
		)

		startTime := time.Now()

		// We need to make sure that for the startTime, the metrics exists in Prometheus.
		// if they don't exist, we increase the startTime and try again.
		// Note: it's important that Polling interval in Eventually is greater than the startTime increment.

		getStartTime := func() time.Time { return startTime }
		modifyStartTime := func() { startTime = startTime.Add(500 * time.Millisecond) }

		queries := []string{
			fmt.Sprintf(`container_memory_usage_bytes{pod="%s",container="nginx-gateway"}`, ngfPodName),
			fmt.Sprintf(`container_cpu_usage_seconds_total{pod="%s",container="nginx-gateway"}`, ngfPodName),
			// We don't need to check all nginx_gateway_fabric_* metrics, as they are collected at the same time
			fmt.Sprintf(`nginx_gateway_fabric_nginx_reloads_total{pod="%s"}`, ngfPodName),
		}

		for _, q := range queries {
			Eventually(
				createMetricExistChecker(
					q,
					getStartTime,
					modifyStartTime,
				),
			).WithTimeout(metricExistTimeout).WithPolling(metricExistPolling).Should(Succeed())
		}

		test()

		// We sleep for 2 scape intervals to ensure that Prometheus scrapes the metrics after the test() finishes
		// before endTime, so that we don't lose any metric values like reloads.
		time.Sleep(2 * scrapeInterval)

		endTime := time.Now()

		// Now we check that Prometheus has the metrics for the endTime

		// If the test duration is small (which can happen if you run the test with small number of resources),
		// the rate query may not return any data.
		// To ensure it returns data, we increase the startTime.
		Eventually(
			createEndTimeFinder(
				fmt.Sprintf(`rate(container_cpu_usage_seconds_total{pod="%s",container="nginx-gateway"}[2m])`, ngfPodName),
				startTime,
				&endTime,
			),
		).WithTimeout(metricExistTimeout).WithPolling(metricExistPolling).Should(Succeed())

		getEndTime := func() time.Time { return endTime }
		noOpModifier := func() {}

		queries = []string{
			fmt.Sprintf(`container_memory_usage_bytes{pod="%s",container="nginx-gateway"}`, ngfPodName),
			// We don't need to check all nginx_gateway_fabric_* metrics, as they are collected at the same time
			fmt.Sprintf(`nginx_gateway_fabric_nginx_reloads_total{pod="%s"}`, ngfPodName),
		}

		for _, q := range queries {
			Eventually(
				createMetricExistChecker(
					q,
					getEndTime,
					noOpModifier,
				),
			).WithTimeout(metricExistTimeout).WithPolling(metricExistPolling).Should(Succeed())
		}

		// Collect metric values
		// For some metrics, generate PNGs

		result, err := promInstance.QueryRange(
			fmt.Sprintf(`container_memory_usage_bytes{pod="%s",container="nginx-gateway"}`, ngfPodName),
			v1.Range{
				Start: startTime,
				End:   endTime,
				Step:  queryRangeStep,
			},
		)
		Expect(err).ToNot(HaveOccurred())

		memCSV := filepath.Join(testResultsDir, "memory.csv")

		Expect(framework.WritePrometheusMatrixToCSVFile(memCSV, result)).To(Succeed())

		Expect(
			framework.GenerateMemoryPNG(testResultsDir, memCSV, "memory.png"),
		).To(Succeed())

		Expect(os.Remove(memCSV)).To(Succeed())

		result, err = promInstance.QueryRange(
			fmt.Sprintf(`rate(container_cpu_usage_seconds_total{pod="%s",container="nginx-gateway"}[2m])`, ngfPodName),
			v1.Range{
				Start: startTime,
				End:   endTime,
				Step:  queryRangeStep,
			},
		)
		Expect(err).ToNot(HaveOccurred())

		cpuCSV := filepath.Join(testResultsDir, "cpu.csv")

		Expect(framework.WritePrometheusMatrixToCSVFile(cpuCSV, result)).To(Succeed())

		Expect(
			framework.GenerateCPUPNG(testResultsDir, cpuCSV, "cpu.png"),
		).To(Succeed())

		Expect(os.Remove(cpuCSV)).To(Succeed())

		reloadCount := getFirstValueOfVector(
			fmt.Sprintf(
				`nginx_gateway_fabric_nginx_reloads_total{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_nginx_reloads_total{pod="%s"} @ %d`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		reloadErrsCount := getFirstValueOfVector(
			fmt.Sprintf(
				`nginx_gateway_fabric_nginx_reload_errors_total{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_nginx_reload_errors_total{pod="%s"} @ %d`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		reloadAvgTime := getFirstValueOfVector(
			fmt.Sprintf(
				`(nginx_gateway_fabric_nginx_reloads_milliseconds_sum{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_nginx_reloads_milliseconds_sum{pod="%s"} @ %d)`+
					` / `+
					`(nginx_gateway_fabric_nginx_reloads_total{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_nginx_reloads_total{pod="%s"} @ %d)`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			))

		reloadBuckets := getBuckets(
			fmt.Sprintf(
				`nginx_gateway_fabric_nginx_reloads_milliseconds_bucket{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_nginx_reloads_milliseconds_bucket{pod="%s"} @ %d`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		eventsCount := getFirstValueOfVector(
			fmt.Sprintf(
				`nginx_gateway_fabric_event_batch_processing_milliseconds_count{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_event_batch_processing_milliseconds_count{pod="%s"} @ %d`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		eventsAvgTime := getFirstValueOfVector(
			fmt.Sprintf(
				`(nginx_gateway_fabric_event_batch_processing_milliseconds_sum{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_event_batch_processing_milliseconds_sum{pod="%s"} @ %d)`+
					` / `+
					`(nginx_gateway_fabric_event_batch_processing_milliseconds_count{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_event_batch_processing_milliseconds_count{pod="%s"} @ %d)`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		eventsBuckets := getBuckets(
			fmt.Sprintf(
				`nginx_gateway_fabric_event_batch_processing_milliseconds_bucket{pod="%s"}`+
					` - `+
					`nginx_gateway_fabric_event_batch_processing_milliseconds_bucket{pod="%s"} @ %d`,
				ngfPodName,
				ngfPodName,
				startTime.Unix(),
			),
		)

		// Check container logs for errors

		ngfErrors := checkLogErrors(
			"nginx-gateway",
			[]string{"error"},
			filepath.Join(testResultsDir, "ngf.log"),
		)
		nginxErrors := checkLogErrors(
			"nginx",
			[]string{"[error]", "[emerg]", "[crit]", "[alert]"},
			filepath.Join(testResultsDir, "nginx.log"),
		)

		// Check container restarts

		pod, err := resourceManager.GetPod(ngfNamespace, ngfPodName)
		Expect(err).ToNot(HaveOccurred())

		findRestarts := func(name string) int {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name == name {
					return int(containerStatus.RestartCount)
				}
			}
			Fail(fmt.Sprintf("container %s not found", name))
			return 0
		}

		ngfRestarts := findRestarts("nginx-gateway")
		nginxRestarts := findRestarts("nginx")

		// Write results

		results := scaleTestResults{
			Name:                   testName,
			ReloadCount:            int(reloadCount),
			ReloadErrsCount:        int(reloadErrsCount),
			ReloadAvgTime:          int(reloadAvgTime),
			ReloadBuckets:          reloadBuckets,
			EventsCount:            int(eventsCount),
			EventsAvgTime:          int(eventsAvgTime),
			EventsBuckets:          eventsBuckets,
			NGFErrors:              ngfErrors,
			NginxErrors:            nginxErrors,
			NGFContainerRestarts:   ngfRestarts,
			NginxContainerRestarts: nginxRestarts,
		}

		err = writeScaleResults(outFile, results)
		Expect(err).ToNot(HaveOccurred())
	}

	runScaleResources := func(objects framework.ScaleObjects, testResultsDir string, protocol string) {
		ttrCsvFile, writer, err := framework.NewCSVResultsWriter(testResultsDir, "ttr.csv")
		Expect(err).ToNot(HaveOccurred())

		Expect(resourceManager.Apply(objects.BaseObjects)).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		Expect(resourceManager.WaitForPodsToBeReady(ctx, ns.Name)).To(Succeed())

		for i := 0; i < len(objects.ScaleIterationGroups); i++ {
			Expect(resourceManager.Apply(objects.ScaleIterationGroups[i])).To(Succeed())

			url := fmt.Sprintf("%s://%d.example.com", protocol, i)

			if protocol == "http" && portFwdPort != 0 {
				url = fmt.Sprintf("%s://%d.example.com:%d", protocol, i, portFwdPort)
			} else if protocol == "https" && portFwdHTTPSPort != 0 {
				url = fmt.Sprintf("%s://%d.example.com:%d", protocol, i, portFwdHTTPSPort)
			}

			startCheck := time.Now()

			Eventually(
				createResponseChecker(url, address),
			).WithTimeout(30 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

			ttr := time.Since(startCheck)

			seconds := ttr.Seconds()
			record := []string{strconv.Itoa(i + 1), strconv.FormatFloat(seconds, 'f', -1, 64)}

			Expect(writer.Write(record)).To(Succeed())
		}

		writer.Flush()
		Expect(ttrCsvFile.Close()).To(Succeed())

		Expect(
			framework.GenerateTTRPNG(testResultsDir, ttrCsvFile.Name(), "ttr.png"),
		).To(Succeed())

		Expect(os.Remove(ttrCsvFile.Name())).To(Succeed())

		Expect(resourceManager.Delete(objects.BaseObjects)).To(Succeed())
		for i := 0; i < len(objects.ScaleIterationGroups); i++ {
			Expect(resourceManager.Delete(objects.ScaleIterationGroups[i])).To(Succeed())
		}
	}

	runScaleUpstreams := func() {
		Expect(resourceManager.ApplyFromFiles(upstreamsManifests, ns.Name)).To(Succeed())
		Expect(resourceManager.WaitForAppsToBeReady(ns.Name)).To(Succeed())

		url := "http://hello.example.com"
		if portFwdPort != 0 {
			url = fmt.Sprintf("http://hello.example.com:%d", portFwdPort)
		}

		Eventually(
			createResponseChecker(url, address),
		).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		Expect(
			resourceManager.ScaleDeployment(ns.Name, "backend", upstreamServerCount),
		).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		Expect(resourceManager.WaitForPodsToBeReady(ctx, ns.Name)).To(Succeed())

		Eventually(
			createResponseChecker(url, address),
		).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		Expect(
			resourceManager.DeleteFromFiles(upstreamsManifests, ns.Name),
		).To(Succeed())
	}

	setNamespace := func(objects framework.ScaleObjects) {
		for _, obj := range objects.BaseObjects {
			obj.SetNamespace(ns.Name)
		}
		for _, objs := range objects.ScaleIterationGroups {
			for _, obj := range objs {
				obj.SetNamespace(ns.Name)
			}
		}
	}

	It(fmt.Sprintf("scales HTTP listeners to %d", httpListenerCount), func() {
		const testName = "TestScale_Listeners"

		testResultsDir := filepath.Join(resultsDir, testName)
		Expect(os.MkdirAll(testResultsDir, 0755)).To(Succeed())

		objects, err := framework.GenerateScaleListenerObjects(httpListenerCount, false /*non-tls*/)
		Expect(err).ToNot(HaveOccurred())

		setNamespace(objects)

		runTestWithMetricsAndLogs(
			testName,
			testResultsDir,
			func() {
				runScaleResources(
					objects,
					testResultsDir,
					"http",
				)
			},
		)

		Expect(
			os.RemoveAll(filepath.Join(testResultsDir, "manifests")),
		).To(Succeed())
	})

	It(fmt.Sprintf("scales HTTPS listeners to %d", httpsListenerCount), func() {
		const testName = "TestScale_HTTPSListeners"

		testResultsDir := filepath.Join(resultsDir, testName)
		Expect(os.MkdirAll(testResultsDir, 0o755)).To(Succeed())

		objects, err := framework.GenerateScaleListenerObjects(httpsListenerCount, true /*tls*/)
		Expect(err).ToNot(HaveOccurred())

		setNamespace(objects)

		runTestWithMetricsAndLogs(
			testName,
			testResultsDir,
			func() {
				runScaleResources(
					objects,
					testResultsDir,
					"https",
				)
			},
		)

		Expect(
			os.RemoveAll(filepath.Join(testResultsDir, "manifests")),
		).To(Succeed())
	})

	It(fmt.Sprintf("scales HTTP routes to %d", httpRouteCount), func() {
		const testName = "TestScale_HTTPRoutes"

		testResultsDir := filepath.Join(resultsDir, testName)
		Expect(os.MkdirAll(testResultsDir, 0o755)).To(Succeed())

		objects, err := framework.GenerateScaleHTTPRouteObjects(httpRouteCount)
		Expect(err).ToNot(HaveOccurred())

		setNamespace(objects)

		runTestWithMetricsAndLogs(
			testName,
			testResultsDir,
			func() {
				runScaleResources(
					objects,
					testResultsDir,
					"http",
				)
			},
		)

		Expect(
			os.RemoveAll(filepath.Join(testResultsDir, "manifests")),
		).To(Succeed())
	})

	It(fmt.Sprintf("scales upstream servers to %d", upstreamServerCount), func() {
		const testName = "TestScale_UpstreamServers"

		testResultsDir := filepath.Join(resultsDir, testName)
		Expect(os.MkdirAll(testResultsDir, 0o755)).To(Succeed())

		runTestWithMetricsAndLogs(
			testName,
			testResultsDir,
			func() {
				runScaleUpstreams()
			},
		)
	})

	It("scale HTTP matches", func() {
		const testName = "TestScale_HTTPMatches"

		Expect(resourceManager.ApplyFromFiles(matchesManifests, ns.Name)).To(Succeed())
		Expect(resourceManager.WaitForAppsToBeReady(ns.Name)).To(Succeed())

		var port int
		if portFwdPort != 0 {
			port = portFwdPort
		} else {
			port = 80
		}

		addr := fmt.Sprintf("%s:%d", address, port)

		baseURL := "http://cafe.example.com"

		text := fmt.Sprintf("\n## Test %s\n\n", testName)

		_, err := fmt.Fprint(outFile, text)
		Expect(err).ToNot(HaveOccurred())

		run := func(t framework.Target) {
			cfg := framework.LoadTestConfig{
				Targets:     []framework.Target{t},
				Rate:        1000,
				Duration:    30 * time.Second,
				Description: "First matches",
				Proxy:       addr,
				ServerName:  "cafe.example.com",
			}
			_, metrics := framework.RunLoadTest(cfg)

			_, err = fmt.Fprintln(outFile, "```text")
			Expect(err).ToNot(HaveOccurred())
			Expect(framework.WriteVegetaResults(outFile, &metrics)).To(Succeed())
			_, err = fmt.Fprintln(outFile, "```")
			Expect(err).ToNot(HaveOccurred())
		}

		run(framework.Target{
			Method: "GET",
			URL:    fmt.Sprintf("%s%s", baseURL, "/latte"),
			Header: map[string][]string{
				"header-1": {"header-1-val"},
			},
		})

		run(framework.Target{
			Method: "GET",
			URL:    fmt.Sprintf("%s%s", baseURL, "/latte"),
			Header: map[string][]string{
				"header-50": {"header-50-val"},
			},
		})

		Expect(resourceManager.DeleteFromFiles(matchesManifests, ns.Name)).To(Succeed())
	})

	AfterEach(func() {
		teardown(releaseName)
	})

	AfterAll(func() {
		close(promPortForwardStopCh)
		Expect(framework.UninstallPrometheus()).To(Succeed())
		Expect(resourceManager.Delete([]client.Object{ns})).To(Succeed())
		Expect(outFile.Close()).To(Succeed())

		// restoring NGF shared among tests in the suite
		cfg := getDefaultSetupCfg()
		labelFilter := GinkgoLabelFilter()
		cfg.nfr = isNFR(labelFilter)
		setup(cfg)
	})
})

type bucket struct {
	Le  string
	Val int
}

type scaleTestResults struct {
	Name string

	ReloadCount     int
	ReloadErrsCount int
	ReloadAvgTime   int
	ReloadBuckets   []bucket

	EventsCount   int
	EventsAvgTime int
	EventsBuckets []bucket

	NGFErrors   int
	NginxErrors int

	NGFContainerRestarts   int
	NginxContainerRestarts int
}

const scaleResultTemplate = `
## Test {{ .Name }}

### Reloads

- Total: {{ .ReloadCount }}
- Total Errors: {{ .ReloadErrsCount }}
- Average Time: {{ .ReloadAvgTime }}ms
- Reload distribution:
{{- range .ReloadBuckets }}
	- {{ .Le }}ms: {{ .Val }}
{{- end }}

### Event Batch Processing

- Total: {{ .EventsCount }}
- Average Time: {{ .EventsAvgTime }}ms
- Event Batch Processing distribution:
{{- range .EventsBuckets }}
	- {{ .Le }}ms: {{ .Val }}
{{- end }}

### Errors

- NGF errors: {{ .NGFErrors }}
- NGF container restarts: {{ .NGFContainerRestarts }}
- NGINX errors: {{ .NginxErrors }}
- NGINX container restarts: {{ .NginxContainerRestarts }}

### Graphs and Logs

See [output directory](./{{ .Name }}) for more details.
The logs are attached only if there are errors.
`

func writeScaleResults(dest io.Writer, results scaleTestResults) error {
	tmpl, err := template.New("results").Parse(scaleResultTemplate)
	if err != nil {
		return err
	}

	return tmpl.Execute(dest, results)
}
