package suite

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nginxinc/nginx-gateway-fabric/tests/framework"
)

var _ = Describe("Dataplane performance", Ordered, Label("performance"), func() {
	files := []string{
		"dp-perf/coffee.yaml",
		"dp-perf/gateway.yaml",
		"dp-perf/cafe-routes.yaml",
	}
	ns := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dp-perf",
		},
	}

	var addr string
	targetURL := "http://cafe.example.com"
	var outFile *os.File

	t1 := framework.Target{
		Method: "GET",
		URL:    fmt.Sprintf("%s%s", targetURL, "/latte"),
	}
	t2 := framework.Target{
		Method: "GET",
		URL:    fmt.Sprintf("%s%s", targetURL, "/coffee"),
		Header: http.Header{"version": []string{"v2"}},
	}
	t3 := framework.Target{
		Method: "GET",
		URL:    fmt.Sprintf("%s%s", targetURL, "/coffee?TEST=v2"),
	}
	t4 := framework.Target{
		Method: "GET",
		URL:    fmt.Sprintf("%s%s", targetURL, "/tea"),
	}
	t5 := framework.Target{
		Method: "POST",
		URL:    fmt.Sprintf("%s%s", targetURL, "/tea"),
	}

	BeforeAll(func() {
		Expect(resourceManager.Apply([]client.Object{ns})).To(Succeed())
		Expect(resourceManager.ApplyFromFiles(files, ns.Name)).To(Succeed())
		Expect(resourceManager.WaitForAppsToBeReady(ns.Name)).To(Succeed())

		port := ":80"
		if portFwdPort != 0 {
			port = fmt.Sprintf(":%s", strconv.Itoa(portFwdPort))
		}
		addr = fmt.Sprintf("%s%s", address, port)

		resultsDir, err := framework.CreateResultsDir("dp-perf", version)
		Expect(err).ToNot(HaveOccurred())

		filename := filepath.Join(resultsDir, fmt.Sprintf("%s.md", version))
		outFile, err = framework.CreateResultsFile(filename)
		Expect(err).ToNot(HaveOccurred())
		Expect(framework.WriteSystemInfoToFile(outFile, clusterInfo)).To(Succeed())
	})

	AfterAll(func() {
		Expect(resourceManager.DeleteFromFiles(files, ns.Name)).To(Succeed())
		Expect(resourceManager.Delete([]client.Object{ns})).To(Succeed())
		outFile.Close()
	})

	DescribeTable("Run each load test",
		func(target framework.Target, description string, counter int) {
			text := fmt.Sprintf("\n## Test%d: %s\n\n```text\n", counter, description)
			_, err := fmt.Fprint(outFile, text)
			Expect(err).ToNot(HaveOccurred())

			_, metrics := framework.RunLoadTest(
				[]framework.Target{target},
				1000,
				30*time.Second,
				description,
				addr,
				"cafe.example.com",
			)

			Expect(framework.WriteResults(outFile, &metrics)).To(Succeed())

			_, err = fmt.Fprint(outFile, "```\n")
			Expect(err).ToNot(HaveOccurred())
		},
		Entry("Running latte path based routing", t1, "Running latte path based routing", 1),
		Entry("Running coffee header based routing", t2, "Running coffee header based routing", 2),
		Entry("Running coffee query based routing", t3, "Running coffee query based routing", 3),
		Entry("Running tea GET method based routing", t4, "Running tea GET method based routing", 4),
		Entry("Running tea POST method based routing", t5, "Running tea POST method based routing", 5),
	)
})
