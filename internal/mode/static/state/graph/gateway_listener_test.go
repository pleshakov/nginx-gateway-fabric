package graph

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/nginx-gateway-fabric/framework/conditions"
	"github.com/nginxinc/nginx-gateway-fabric/framework/helpers"
	staticConds "github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/conditions"
)

func TestValidateHTTPListener(t *testing.T) {
	protectedPorts := ProtectedPorts{9113: "MetricsPort"}

	tests := []struct {
		l        v1.Listener
		name     string
		expected []conditions.Condition
	}{
		{
			l: v1.Listener{
				Port: 80,
			},
			expected: nil,
			name:     "valid",
		},
		{
			l: v1.Listener{
				Port: 0,
			},
			expected: staticConds.NewListenerUnsupportedValue(`port: Invalid value: 0: port must be between 1-65535`),
			name:     "invalid port",
		},
		{
			l: v1.Listener{
				Port: 80,
				TLS: &v1.GatewayTLSConfig{
					Mode: helpers.GetPointer(v1.TLSModeTerminate),
				},
				Name: "http-listener",
			},
			expected: staticConds.NewListenerUnsupportedValue(`tls: Forbidden: tls is not supported for HTTP listener`),
			name:     "invalid HTTP listener with TLS",
		},
		{
			l: v1.Listener{
				Port: 9113,
			},
			expected: staticConds.NewListenerUnsupportedValue(
				`port: Invalid value: 9113: port is already in use as MetricsPort`,
			),
			name: "invalid protected port",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			v := createHTTPListenerValidator(protectedPorts)

			result, attachable := v(test.l)

			g.Expect(result).To(Equal(test.expected))
			g.Expect(attachable).To(BeTrue())
		})
	}
}

func TestValidateHTTPSListener(t *testing.T) {
	secretNs := "secret-ns"

	validSecretRef := v1.SecretObjectReference{
		Kind:      (*v1.Kind)(helpers.GetPointer("Secret")),
		Name:      "secret",
		Namespace: (*v1.Namespace)(helpers.GetPointer(secretNs)),
	}

	invalidSecretRefGroup := v1.SecretObjectReference{
		Group:     (*v1.Group)(helpers.GetPointer("some-group")),
		Kind:      (*v1.Kind)(helpers.GetPointer("Secret")),
		Name:      "secret",
		Namespace: (*v1.Namespace)(helpers.GetPointer(secretNs)),
	}

	invalidSecretRefKind := v1.SecretObjectReference{
		Kind:      (*v1.Kind)(helpers.GetPointer("ConfigMap")),
		Name:      "secret",
		Namespace: (*v1.Namespace)(helpers.GetPointer(secretNs)),
	}

	protectedPorts := ProtectedPorts{9113: "MetricsPort"}

	tests := []struct {
		l        v1.Listener
		name     string
		expected []conditions.Condition
	}{
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef},
				},
			},
			expected: nil,
			name:     "valid",
		},
		{
			l: v1.Listener{
				Port: 0,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef},
				},
			},
			expected: staticConds.NewListenerUnsupportedValue(`port: Invalid value: 0: port must be between 1-65535`),
			name:     "invalid port",
		},
		{
			l: v1.Listener{
				Port: 9113,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef},
				},
			},
			expected: staticConds.NewListenerUnsupportedValue(
				`port: Invalid value: 9113: port is already in use as MetricsPort`,
			),
			name: "invalid protected port",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef},
					Options:         map[v1.AnnotationKey]v1.AnnotationValue{"key": "val"},
				},
			},
			expected: staticConds.NewListenerUnsupportedValue("tls.options: Forbidden: options are not supported"),
			name:     "invalid options",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModePassthrough),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef},
				},
			},
			expected: staticConds.NewListenerUnsupportedValue(
				`tls.mode: Unsupported value: "Passthrough": supported values: "Terminate"`,
			),
			name: "invalid tls mode",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS:  nil,
			},
			expected: staticConds.NewListenerUnsupportedValue(
				`TLS: Required value: tls must be defined for HTTPS listener`,
			),
			name: "nil tls",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{invalidSecretRefGroup},
				},
			},
			expected: staticConds.NewListenerInvalidCertificateRef(
				`tls.certificateRefs[0].group: Unsupported value: "some-group": supported values: ""`,
			),
			name: "invalid cert ref group",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{},
				},
			},
			expected: staticConds.NewListenerInvalidCertificateRef(
				`tls.certificateRefs: Required value: certificateRefs must be defined for TLS mode terminate`,
			),
			name: "zero cert refs",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{invalidSecretRefKind},
				},
			},
			expected: staticConds.NewListenerInvalidCertificateRef(
				`tls.certificateRefs[0].kind: Unsupported value: "ConfigMap": supported values: "Secret"`,
			),
			name: "invalid cert ref kind",
		},
		{
			l: v1.Listener{
				Port: 443,
				TLS: &v1.GatewayTLSConfig{
					Mode:            helpers.GetPointer(v1.TLSModeTerminate),
					CertificateRefs: []v1.SecretObjectReference{validSecretRef, validSecretRef},
				},
			},
			expected: staticConds.NewListenerUnsupportedValue(
				"tls.certificateRefs: Too many: 2: must have at most 1 items",
			),
			name: "too many cert refs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			v := createHTTPSListenerValidator(protectedPorts)

			result, attachable := v(test.l)
			g.Expect(result).To(Equal(test.expected))
			g.Expect(attachable).To(BeTrue())
		})
	}
}

func TestValidateListenerHostname(t *testing.T) {
	tests := []struct {
		hostname  *v1.Hostname
		name      string
		expectErr bool
	}{
		{
			hostname:  nil,
			expectErr: false,
			name:      "nil hostname",
		},
		{
			hostname:  (*v1.Hostname)(helpers.GetPointer("")),
			expectErr: false,
			name:      "empty hostname",
		},
		{
			hostname:  (*v1.Hostname)(helpers.GetPointer("foo.example.com")),
			expectErr: false,
			name:      "valid hostname",
		},
		{
			hostname:  (*v1.Hostname)(helpers.GetPointer("*.example.com")),
			expectErr: false,
			name:      "wildcard hostname",
		},
		{
			hostname:  (*v1.Hostname)(helpers.GetPointer("example$com")),
			expectErr: true,
			name:      "invalid hostname",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			conds, attachable := validateListenerHostname(v1.Listener{Hostname: test.hostname})

			if test.expectErr {
				g.Expect(conds).ToNot(BeEmpty())
				g.Expect(attachable).To(BeFalse())
			} else {
				g.Expect(conds).To(BeEmpty())
				g.Expect(attachable).To(BeTrue())
			}
		})
	}
}

func TestGetAndValidateListenerSupportedKinds(t *testing.T) {
	HTTPRouteGroupKind := []v1.RouteGroupKind{
		{
			Kind:  "HTTPRoute",
			Group: helpers.GetPointer[v1.Group](v1.GroupName),
		},
	}
	TCPRouteGroupKind := []v1.RouteGroupKind{
		{
			Kind:  "TCPRoute",
			Group: helpers.GetPointer[v1.Group](v1.GroupName),
		},
	}
	tests := []struct {
		protocol  v1.ProtocolType
		name      string
		kind      []v1.RouteGroupKind
		expected  []v1.RouteGroupKind
		expectErr bool
	}{
		{
			protocol:  v1.TCPProtocolType,
			expectErr: false,
			name:      "unsupported protocol is ignored",
			kind:      TCPRouteGroupKind,
			expected:  []v1.RouteGroupKind{},
		},
		{
			protocol: v1.HTTPProtocolType,
			kind: []v1.RouteGroupKind{
				{
					Kind:  "HTTPRoute",
					Group: helpers.GetPointer[v1.Group]("bad-group"),
				},
			},
			expectErr: true,
			name:      "invalid group",
			expected:  []v1.RouteGroupKind{},
		},
		{
			protocol:  v1.HTTPProtocolType,
			kind:      TCPRouteGroupKind,
			expectErr: true,
			name:      "invalid kind",
			expected:  []v1.RouteGroupKind{},
		},
		{
			protocol:  v1.HTTPProtocolType,
			kind:      HTTPRouteGroupKind,
			expectErr: false,
			name:      "valid HTTP",
			expected:  HTTPRouteGroupKind,
		},
		{
			protocol:  v1.HTTPSProtocolType,
			kind:      HTTPRouteGroupKind,
			expectErr: false,
			name:      "valid HTTPS",
			expected:  HTTPRouteGroupKind,
		},
		{
			protocol:  v1.HTTPSProtocolType,
			expectErr: false,
			name:      "valid HTTPS no kind specified",
			expected: []v1.RouteGroupKind{
				{
					Kind: "HTTPRoute",
				},
			},
		},
		{
			protocol: v1.HTTPProtocolType,
			kind: []v1.RouteGroupKind{
				{
					Kind:  "HTTPRoute",
					Group: helpers.GetPointer[v1.Group](v1.GroupName),
				},
				{
					Kind:  "bad-kind",
					Group: helpers.GetPointer[v1.Group](v1.GroupName),
				},
			},
			expectErr: true,
			name:      "valid and invalid kinds",
			expected:  HTTPRouteGroupKind,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			listener := v1.Listener{
				Protocol: test.protocol,
			}

			if test.kind != nil {
				listener.AllowedRoutes = &v1.AllowedRoutes{
					Kinds: test.kind,
				}
			}

			conds, kinds := getAndValidateListenerSupportedKinds(listener)
			g.Expect(helpers.Diff(test.expected, kinds)).To(BeEmpty())
			if test.expectErr {
				g.Expect(conds).ToNot(BeEmpty())
			} else {
				g.Expect(conds).To(BeEmpty())
			}
		})
	}
}

func TestValidateListenerLabelSelector(t *testing.T) {
	tests := []struct {
		selector  *metav1.LabelSelector
		from      v1.FromNamespaces
		name      string
		expectErr bool
	}{
		{
			from:      v1.NamespacesFromSelector,
			selector:  &metav1.LabelSelector{},
			expectErr: false,
			name:      "valid spec",
		},
		{
			from:      v1.NamespacesFromSelector,
			selector:  nil,
			expectErr: true,
			name:      "invalid spec",
		},
		{
			from:      v1.NamespacesFromAll,
			selector:  nil,
			expectErr: false,
			name:      "ignored from type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			// create iteration variable inside the loop to fix implicit memory aliasing
			from := test.from

			listener := v1.Listener{
				AllowedRoutes: &v1.AllowedRoutes{
					Namespaces: &v1.RouteNamespaces{
						From:     &from,
						Selector: test.selector,
					},
				},
			}

			conds, attachable := validateListenerLabelSelector(listener)
			if test.expectErr {
				g.Expect(conds).ToNot(BeEmpty())
				g.Expect(attachable).To(BeFalse())
			} else {
				g.Expect(conds).To(BeEmpty())
				g.Expect(attachable).To(BeTrue())
			}
		})
	}
}

func TestValidateListenerPort(t *testing.T) {
	validPorts := []v1.PortNumber{1, 80, 443, 1000, 50000, 65535}
	invalidPorts := []v1.PortNumber{-1, 0, 65536, 80000, 9113}
	protectedPorts := ProtectedPorts{9113: "MetricsPort"}

	for _, p := range validPorts {
		t.Run(fmt.Sprintf("valid port %d", p), func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(validateListenerPort(p, protectedPorts)).To(Succeed())
		})
	}

	for _, p := range invalidPorts {
		t.Run(fmt.Sprintf("invalid port %d", p), func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(validateListenerPort(p, protectedPorts)).ToNot(Succeed())
		})
	}
}
