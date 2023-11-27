package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctlrZap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/provisioner"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/config"
)

const (
	domain                = "gateway.nginx.org"
	gatewayClassFlag      = "gatewayclass"
	gatewayClassNameUsage = `The name of the GatewayClass resource. ` +
		`Every NGINX Gateway Fabric must have a unique corresponding GatewayClass resource.`
	gatewayCtlrNameFlag     = "gateway-ctlr-name"
	gatewayCtlrNameUsageFmt = `The name of the Gateway controller. ` +
		`The controller name must be of the form: DOMAIN/PATH. The controller's domain is '%s'`
)

func createRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "gateway",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return rootCmd
}

func createStaticModeCommand() *cobra.Command {
	// flag names
	const (
		gatewayFlag                = "gateway"
		configFlag                 = "config"
		serviceFlag                = "service"
		updateGCStatusFlag         = "update-gatewayclass-status"
		metricsDisableFlag         = "metrics-disable"
		metricsSecureFlag          = "metrics-secure-serving"
		metricsPortFlag            = "metrics-port"
		healthDisableFlag          = "health-disable"
		healthPortFlag             = "health-port"
		leaderElectionDisableFlag  = "leader-election-disable"
		leaderElectionLockNameFlag = "leader-election-lock-name"
	)

	// flag values
	var (
		gatewayCtlrName = stringValidatingValue{
			validator: validateGatewayControllerName,
		}

		gatewayClassName = stringValidatingValue{
			validator: validateResourceName,
		}

		updateGCStatus bool
		gateway        = namespacedNameValue{}
		configName     = stringValidatingValue{
			validator: validateResourceName,
		}
		serviceName = stringValidatingValue{
			validator: validateResourceName,
		}
		disableMetrics    bool
		metricsSecure     bool
		metricsListenPort = intValidatingValue{
			validator: validatePort,
			value:     9113,
		}
		disableHealth    bool
		healthListenPort = intValidatingValue{
			validator: validatePort,
			value:     8081,
		}

		disableLeaderElection  bool
		leaderElectionLockName = stringValidatingValue{
			validator: validateResourceName,
			value:     "nginx-gateway-leader-election-lock",
		}
	)

	cmd := &cobra.Command{
		Use:   "static-mode",
		Short: "Configure NGINX in the scope of a single Gateway resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			atom := zap.NewAtomicLevel()

			logger := ctlrZap.New(ctlrZap.Level(atom))
			logger.Info(
				"Starting NGINX Gateway Fabric in static mode",
				"version", version,
				"commit", commit,
				"date", date,
			)
			log.SetLogger(logger)

			if err := ensureNoPortCollisions(metricsListenPort.value, healthListenPort.value); err != nil {
				return fmt.Errorf("error validating ports: %w", err)
			}

			podIP := os.Getenv("POD_IP")
			if err := validateIP(podIP); err != nil {
				return fmt.Errorf("error validating POD_IP environment variable: %w", err)
			}

			namespace := os.Getenv("POD_NAMESPACE")
			if namespace == "" {
				return errors.New("POD_NAMESPACE environment variable must be set")
			}

			podName := os.Getenv("POD_NAME")
			if podName == "" {
				return errors.New("POD_NAME environment variable must be set")
			}

			var gwNsName *types.NamespacedName
			if cmd.Flags().Changed(gatewayFlag) {
				gwNsName = &gateway.value
			}

			conf := config.Config{
				GatewayCtlrName:          gatewayCtlrName.value,
				ConfigName:               configName.String(),
				Logger:                   logger,
				AtomicLevel:              atom,
				GatewayClassName:         gatewayClassName.value,
				GatewayNsName:            gwNsName,
				UpdateGatewayClassStatus: updateGCStatus,
				GatewayPodConfig: config.GatewayPodConfig{
					PodIP:       podIP,
					ServiceName: serviceName.value,
					Namespace:   namespace,
				},
				HealthConfig: config.HealthConfig{
					Enabled: !disableHealth,
					Port:    healthListenPort.value,
				},
				MetricsConfig: config.MetricsConfig{
					Enabled: !disableMetrics,
					Port:    metricsListenPort.value,
					Secure:  metricsSecure,
				},
				LeaderElection: config.LeaderElection{
					Enabled:  !disableLeaderElection,
					LockName: leaderElectionLockName.String(),
					Identity: podName,
				},
			}

			if err := static.StartManager(conf); err != nil {
				return fmt.Errorf("failed to start control loop: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Var(
		&gatewayCtlrName,
		gatewayCtlrNameFlag,
		fmt.Sprintf(gatewayCtlrNameUsageFmt, domain),
	)
	utilruntime.Must(cmd.MarkFlagRequired(gatewayCtlrNameFlag))

	cmd.Flags().Var(
		&gatewayClassName,
		gatewayClassFlag,
		gatewayClassNameUsage,
	)
	utilruntime.Must(cmd.MarkFlagRequired(gatewayClassFlag))

	cmd.Flags().Var(
		&gateway,
		gatewayFlag,
		"The namespaced name of the Gateway resource to use. "+
			"Must be of the form: NAMESPACE/NAME. "+
			"If not specified, the control plane will process all Gateways for the configured GatewayClass. "+
			"However, among them, it will choose the oldest resource by creation timestamp. If the timestamps are "+
			"equal, it will choose the resource that appears first in alphabetical order by {namespace}/{name}.",
	)

	cmd.Flags().VarP(
		&configName,
		configFlag,
		"c",
		`The name of the NginxGateway resource to be used for this controller's dynamic configuration.`+
			` Lives in the same Namespace as the controller.`,
	)

	cmd.Flags().Var(
		&serviceName,
		serviceFlag,
		`The name of the Service that fronts this NGINX Gateway Fabric Pod.`+
			` Lives in the same Namespace as the controller.`,
	)

	cmd.Flags().BoolVar(
		&updateGCStatus,
		updateGCStatusFlag,
		true,
		"Update the status of the GatewayClass resource.",
	)

	cmd.Flags().BoolVar(
		&disableMetrics,
		metricsDisableFlag,
		false,
		"Disable exposing metrics in the Prometheus format.",
	)

	cmd.Flags().Var(
		&metricsListenPort,
		metricsPortFlag,
		"Set the port where the metrics are exposed. Format: [1024 - 65535]",
	)

	cmd.Flags().BoolVar(
		&metricsSecure,
		metricsSecureFlag,
		false,
		"Enable serving metrics via https. By default metrics are served via http."+
			" Please note that this endpoint will be secured with a self-signed certificate.",
	)

	cmd.Flags().BoolVar(
		&disableHealth,
		healthDisableFlag,
		false,
		"Disable running the health probe server.",
	)

	cmd.Flags().Var(
		&healthListenPort,
		healthPortFlag,
		"Set the port where the health probe server is exposed. Format: [1024 - 65535]",
	)

	cmd.Flags().BoolVar(
		&disableLeaderElection,
		leaderElectionDisableFlag,
		false,
		"Disable leader election. Leader election is used to avoid multiple replicas of the NGINX Gateway Fabric"+
			" reporting the status of the Gateway API resources. If disabled, "+
			"all replicas of NGINX Gateway Fabric will update the statuses of the Gateway API resources.",
	)

	cmd.Flags().Var(
		&leaderElectionLockName,
		leaderElectionLockNameFlag,
		"The name of the leader election lock. "+
			"A Lease object with this name will be created in the same Namespace as the controller.",
	)

	return cmd
}

func createProvisionerModeCommand() *cobra.Command {
	var (
		gatewayCtlrName = stringValidatingValue{
			validator: validateGatewayControllerName,
		}
		gatewayClassName = stringValidatingValue{
			validator: validateResourceName,
		}
	)

	cmd := &cobra.Command{
		Use:    "provisioner-mode",
		Short:  "Provision a static-mode NGINX Gateway Fabric Deployment per Gateway resource",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := ctlrZap.New()
			logger.Info(
				"Starting NGINX Gateway Fabric Provisioner",
				"version", version,
				"commit", commit,
				"date", date,
			)

			return provisioner.StartManager(provisioner.Config{
				Logger:           logger,
				GatewayClassName: gatewayClassName.value,
				GatewayCtlrName:  gatewayCtlrName.value,
			})
		},
	}

	cmd.Flags().Var(
		&gatewayCtlrName,
		gatewayCtlrNameFlag,
		fmt.Sprintf(gatewayCtlrNameUsageFmt, domain),
	)
	utilruntime.Must(cmd.MarkFlagRequired(gatewayCtlrNameFlag))

	cmd.Flags().Var(
		&gatewayClassName,
		gatewayClassFlag,
		gatewayClassNameUsage,
	)
	utilruntime.Must(cmd.MarkFlagRequired(gatewayClassFlag))

	return cmd
}

// FIXME(pleshakov): Remove this command once NGF min supported Kubernetes version supports sleep action in
// preStop hook.
// nolint:lll
// See https://github.com/kubernetes/enhancements/tree/4ec371d92dcd4f56a2ab18c8ba20bb85d8d20efe/keps/sig-node/3960-pod-lifecycle-sleep-action
func createSleepCommand() *cobra.Command {
	// flag names
	const durationFlag = "duration"
	// flag values
	var duration time.Duration

	cmd := &cobra.Command{
		Use:   "sleep",
		Short: "Sleep for specified duration and exit",
		Run: func(cmd *cobra.Command, args []string) {
			// It is expected that this command is run from lifecycle hook.
			// Because logs from hooks are not visible in the container logs, we don't log here at all.

			logger := ctlrZap.New()

			static.ReportAndSleep(logger)

			time.Sleep(duration)
		},
	}

	cmd.Flags().DurationVar(
		&duration,
		durationFlag,
		30*time.Second,
		"Set the duration of sleep. Must be parsable by https://pkg.go.dev/time#ParseDuration",
	)

	return cmd
}
