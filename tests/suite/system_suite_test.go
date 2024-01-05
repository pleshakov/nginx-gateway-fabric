package suite

import (
	"context"
	"embed"
	"flag"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	coordination "k8s.io/api/coordination/v1"
	core "k8s.io/api/core/v1"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/nginx-gateway-fabric/tests/framework"
)

func TestNGF(t *testing.T) {
	flag.Parse()
	if *gatewayAPIVersion == "" {
		panic("Gateway API version must be set")
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "NGF System Tests")
}

var (
	gatewayAPIVersion = flag.String("gateway-api-version", "", "Version of Gateway API to install")
	k8sVersion        = flag.String("k8s-version", "latest", "Version of k8s being tested on")
	// Configurable NGF installation variables. Helm values will be used as defaults if not specified.
	ngfImageRepository   = flag.String("ngf-image-repo", "", "Image repo for NGF control plane")
	nginxImageRepository = flag.String("nginx-image-repo", "", "Image repo for NGF data plane")
	imageTag             = flag.String("image-tag", "", "Image tag for NGF images")
	imagePullPolicy      = flag.String("pull-policy", "", "Image pull policy for NGF images")
	serviceType          = flag.String("service-type", "NodePort", "Type of service fronting NGF to be deployed")
	isGKEInternalLB      = flag.Bool("is-gke-internal-lb", false, "Is the LB service GKE internal only")
)

var (
	//go:embed manifests/*
	manifests         embed.FS
	k8sClient         client.Client
	resourceManager   framework.ResourceManager
	portForwardStopCh = make(chan struct{}, 1)
	portFwdPort       int
	timeoutConfig     framework.TimeoutConfig
	localChartPath    string
	address           string
	version           string
	clusterInfo       framework.ClusterInfo
)

const (
	releaseName  = "ngf-test"
	ngfNamespace = "nginx-gateway"
)

type setupConfig struct {
	chartPath string
	deploy    bool
}

func setup(cfg setupConfig, extraInstallArgs ...string) {
	log.SetLogger(GinkgoLogr)

	k8sConfig := ctlr.GetConfigOrDie()
	scheme := k8sRuntime.NewScheme()
	Expect(core.AddToScheme(scheme)).To(Succeed())
	Expect(apps.AddToScheme(scheme)).To(Succeed())
	Expect(apiext.AddToScheme(scheme)).To(Succeed())
	Expect(coordination.AddToScheme(scheme)).To(Succeed())
	Expect(v1.AddToScheme(scheme)).To(Succeed())

	options := client.Options{
		Scheme: scheme,
	}

	var err error
	k8sClient, err = client.New(k8sConfig, options)
	Expect(err).ToNot(HaveOccurred())

	timeoutConfig = framework.DefaultTimeoutConfig()
	resourceManager = framework.ResourceManager{
		K8sClient:     k8sClient,
		FS:            manifests,
		TimeoutConfig: timeoutConfig,
	}

	clusterInfo, err = resourceManager.GetClusterInfo()
	Expect(err).ToNot(HaveOccurred())

	if !cfg.deploy {
		return
	}

	installCfg := framework.InstallationConfig{
		ReleaseName:     releaseName,
		Namespace:       ngfNamespace,
		ChartPath:       cfg.chartPath,
		ServiceType:     *serviceType,
		IsGKEInternalLB: *isGKEInternalLB,
	}

	// if we aren't installing from the public charts, then set the custom images
	if !strings.HasPrefix(cfg.chartPath, "oci://") {
		installCfg.NgfImageRepository = *ngfImageRepository
		installCfg.NginxImageRepository = *nginxImageRepository
		installCfg.ImageTag = *imageTag
		installCfg.ImagePullPolicy = *imagePullPolicy
	}

	if *imageTag != "" {
		version = *imageTag
	} else {
		version = "edge"
	}

	output, err := framework.InstallGatewayAPI(k8sClient, *gatewayAPIVersion, *k8sVersion)
	Expect(err).ToNot(HaveOccurred(), string(output))

	output, err = framework.InstallNGF(installCfg, extraInstallArgs...)
	Expect(err).ToNot(HaveOccurred(), string(output))

	podNames, err := framework.GetNGFPodNames(
		k8sClient,
		installCfg.Namespace,
		installCfg.ReleaseName,
		timeoutConfig.CreateTimeout,
	)
	Expect(err).ToNot(HaveOccurred())

	if *serviceType != "LoadBalancer" {
		portFwdPort, err = framework.PortForward(k8sConfig, installCfg.Namespace, podNames[0], portForwardStopCh)
		address = "127.0.0.1"
	} else {
		address, err = resourceManager.GetLBIPAddress(installCfg.Namespace)
	}
	Expect(err).ToNot(HaveOccurred())
}

func teardown() {
	if portFwdPort != 0 {
		portForwardStopCh <- struct{}{}
	}

	cfg := framework.InstallationConfig{
		ReleaseName: releaseName,
		Namespace:   ngfNamespace,
	}

	output, err := framework.UninstallNGF(cfg, k8sClient)
	Expect(err).ToNot(HaveOccurred(), string(output))

	output, err = framework.UninstallGatewayAPI(*gatewayAPIVersion, *k8sVersion)
	Expect(err).ToNot(HaveOccurred(), string(output))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	Expect(wait.PollUntilContextCancel(
		ctx,
		500*time.Millisecond,
		true, /* poll immediately */
		func(ctx context.Context) (bool, error) {
			key := types.NamespacedName{Name: ngfNamespace}
			if err := k8sClient.Get(ctx, key, &core.Namespace{}); err != nil && apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		},
	)).To(Succeed())
}

var _ = BeforeSuite(func() {
	_, file, _, _ := runtime.Caller(0)
	fileDir := path.Join(path.Dir(file), "../")
	basepath := filepath.Dir(fileDir)
	localChartPath = filepath.Join(basepath, "deploy/helm-chart")

	cfg := setupConfig{
		chartPath: localChartPath,
		deploy:    true,
	}

	// If we are running the upgrade test only, then skip the initial deployment.
	// The upgrade test will deploy its own version of NGF.
	suiteConfig, _ := GinkgoConfiguration()
	if suiteConfig.LabelFilter == "upgrade" {
		cfg.deploy = false
	}

	setup(cfg)
})

var _ = AfterSuite(func() {
	teardown()
})
