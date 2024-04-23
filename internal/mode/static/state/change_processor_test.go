package state_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	apiv1 "k8s.io/api/core/v1"
	discoveryV1 "k8s.io/api/discovery/v1"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-gateway-fabric/framework/conditions"
	"github.com/nginxinc/nginx-gateway-fabric/framework/controller/index"
	"github.com/nginxinc/nginx-gateway-fabric/framework/gatewayclass"
	"github.com/nginxinc/nginx-gateway-fabric/framework/helpers"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state"
	staticConds "github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/conditions"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/graph"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/validation"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/validation/validationfakes"
)

const (
	controllerName = "my.controller"
	gcName         = "test-class"
)

func createRoute(
	name string,
	gateway string,
	hostname string,
	backendRefs ...v1.HTTPBackendRef,
) *v1.HTTPRoute {
	return &v1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "test",
			Name:       name,
			Generation: 1,
		},
		Spec: v1.HTTPRouteSpec{
			CommonRouteSpec: v1.CommonRouteSpec{
				ParentRefs: []v1.ParentReference{
					{
						Namespace: (*v1.Namespace)(helpers.GetPointer("test")),
						Name:      v1.ObjectName(gateway),
						SectionName: (*v1.SectionName)(
							helpers.GetPointer("listener-80-1"),
						),
					},
					{
						Namespace: (*v1.Namespace)(helpers.GetPointer("test")),
						Name:      v1.ObjectName(gateway),
						SectionName: (*v1.SectionName)(
							helpers.GetPointer("listener-443-1"),
						),
					},
				},
			},
			Hostnames: []v1.Hostname{
				v1.Hostname(hostname),
			},
			Rules: []v1.HTTPRouteRule{
				{
					Matches: []v1.HTTPRouteMatch{
						{
							Path: &v1.HTTPPathMatch{
								Type:  helpers.GetPointer(v1.PathMatchPathPrefix),
								Value: helpers.GetPointer("/"),
							},
						},
					},
					BackendRefs: backendRefs,
				},
			},
		},
	}
}

func createGateway(name string) *v1.Gateway {
	return &v1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "test",
			Name:       name,
			Generation: 1,
		},
		Spec: v1.GatewaySpec{
			GatewayClassName: gcName,
			Listeners: []v1.Listener{
				{
					Name:     "listener-80-1",
					Hostname: nil,
					Port:     80,
					Protocol: v1.HTTPProtocolType,
				},
			},
		},
	}
}

func createGatewayWithTLSListener(name string, tlsSecret *apiv1.Secret) *v1.Gateway {
	gw := createGateway(name)

	l := v1.Listener{
		Name:     "listener-443-1",
		Hostname: nil,
		Port:     443,
		Protocol: v1.HTTPSProtocolType,
		TLS: &v1.GatewayTLSConfig{
			Mode: helpers.GetPointer(v1.TLSModeTerminate),
			CertificateRefs: []v1.SecretObjectReference{
				{
					Kind:      (*v1.Kind)(helpers.GetPointer("Secret")),
					Name:      v1.ObjectName(tlsSecret.Name),
					Namespace: (*v1.Namespace)(&tlsSecret.Namespace),
				},
			},
		},
	}
	gw.Spec.Listeners = append(gw.Spec.Listeners, l)

	return gw
}

func createRouteWithMultipleRules(
	name, gateway, hostname string,
	rules []v1.HTTPRouteRule,
) *v1.HTTPRoute {
	hr := createRoute(name, gateway, hostname)
	hr.Spec.Rules = rules

	return hr
}

func createHTTPRule(path string, backendRefs ...v1.HTTPBackendRef) v1.HTTPRouteRule {
	return v1.HTTPRouteRule{
		Matches: []v1.HTTPRouteMatch{
			{
				Path: &v1.HTTPPathMatch{
					Type:  helpers.GetPointer(v1.PathMatchPathPrefix),
					Value: &path,
				},
			},
		},
		BackendRefs: backendRefs,
	}
}

func createBackendRef(
	kind *v1.Kind,
	name v1.ObjectName,
	namespace *v1.Namespace,
) v1.HTTPBackendRef {
	return v1.HTTPBackendRef{
		BackendRef: v1.BackendRef{
			BackendObjectReference: v1.BackendObjectReference{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
				Port:      helpers.GetPointer[v1.PortNumber](80),
			},
		},
	}
}

func createAlwaysValidValidators() validation.Validators {
	http := &validationfakes.FakeHTTPFieldsValidator{}

	return validation.Validators{
		HTTPFieldsValidator: http,
	}
}

func createScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha2.AddToScheme(scheme))
	utilruntime.Must(apiv1.AddToScheme(scheme))
	utilruntime.Must(discoveryV1.AddToScheme(scheme))
	utilruntime.Must(apiext.AddToScheme(scheme))

	return scheme
}

func getListenerByName(gw *graph.Gateway, name string) *graph.Listener {
	for _, l := range gw.Listeners {
		if l.Name == name {
			return l
		}
	}

	return nil
}

var (
	cert = []byte(`-----BEGIN CERTIFICATE-----
MIIDLjCCAhYCCQDAOF9tLsaXWjANBgkqhkiG9w0BAQsFADBaMQswCQYDVQQGEwJV
UzELMAkGA1UECAwCQ0ExITAfBgNVBAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0
ZDEbMBkGA1UEAwwSY2FmZS5leGFtcGxlLmNvbSAgMB4XDTE4MDkxMjE2MTUzNVoX
DTIzMDkxMTE2MTUzNVowWDELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNBMSEwHwYD
VQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxGTAXBgNVBAMMEGNhZmUuZXhh
bXBsZS5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCp6Kn7sy81
p0juJ/cyk+vCAmlsfjtFM2muZNK0KtecqG2fjWQb55xQ1YFA2XOSwHAYvSdwI2jZ
ruW8qXXCL2rb4CZCFxwpVECrcxdjm3teViRXVsYImmJHPPSyQgpiobs9x7DlLc6I
BA0ZjUOyl0PqG9SJexMV73WIIa5rDVSF2r4kSkbAj4Dcj7LXeFlVXH2I5XwXCptC
n67JCg42f+k8wgzcRVp8XZkZWZVjwq9RUKDXmFB2YyN1XEWdZ0ewRuKYUJlsm692
skOrKQj0vkoPn41EE/+TaVEpqLTRoUY3rzg7DkdzfdBizFO2dsPNFx2CW0jXkNLv
Ko25CZrOhXAHAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAKHFCcyOjZvoHswUBMdL
RdHIb383pWFynZq/LuUovsVA58B0Cg7BEfy5vWVVrq5RIkv4lZ81N29x21d1JH6r
jSnQx+DXCO/TJEV5lSCUpIGzEUYaUPgRyjsM/NUdCJ8uHVhZJ+S6FA+CnOD9rn2i
ZBePCI5rHwEXwnnl8ywij3vvQ5zHIuyBglWr/Qyui9fjPpwWUvUm4nv5SMG9zCV7
PpuwvuatqjO1208BjfE/cZHIg8Hw9mvW9x9C+IQMIMDE7b/g6OcK7LGTLwlFxvA8
7WjEequnayIphMhKRXVf1N349eN98Ez38fOTHTPbdJjFA/PcC+Gyme+iGt5OQdFh
yRE=
-----END CERTIFICATE-----`)
	key = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAqeip+7MvNadI7if3MpPrwgJpbH47RTNprmTStCrXnKhtn41k
G+ecUNWBQNlzksBwGL0ncCNo2a7lvKl1wi9q2+AmQhccKVRAq3MXY5t7XlYkV1bG
CJpiRzz0skIKYqG7Pcew5S3OiAQNGY1DspdD6hvUiXsTFe91iCGuaw1Uhdq+JEpG
wI+A3I+y13hZVVx9iOV8FwqbQp+uyQoONn/pPMIM3EVafF2ZGVmVY8KvUVCg15hQ
dmMjdVxFnWdHsEbimFCZbJuvdrJDqykI9L5KD5+NRBP/k2lRKai00aFGN684Ow5H
c33QYsxTtnbDzRcdgltI15DS7yqNuQmazoVwBwIDAQABAoIBAQCPSdSYnQtSPyql
FfVFpTOsoOYRhf8sI+ibFxIOuRauWehhJxdm5RORpAzmCLyL5VhjtJme223gLrw2
N99EjUKb/VOmZuDsBc6oCF6QNR58dz8cnORTewcotsJR1pn1hhlnR5HqJJBJask1
ZEnUQfcXZrL94lo9JH3E+Uqjo1FFs8xxE8woPBqjZsV7pRUZgC3LhxnwLSExyFo4
cxb9SOG5OmAJozStFoQ2GJOes8rJ5qfdvytgg9xbLaQL/x0kpQ62BoFMBDdqOePW
KfP5zZ6/07/vpj48yA1Q32PzobubsBLd3Kcn32jfm1E7prtWl+JeOFiOznBQFJbN
4qPVRz5hAoGBANtWyxhNCSLu4P+XgKyckljJ6F5668fNj5CzgFRqJ09zn0TlsNro
FTLZcxDqnR3HPYM42JERh2J/qDFZynRQo3cg3oeivUdBVGY8+FI1W0qdub/L9+yu
edOZTQ5XmGGp6r6jexymcJim/OsB3ZnYOpOrlD7SPmBvzNLk4MF6gxbXAoGBAMZO
0p6HbBmcP0tjFXfcKE77ImLm0sAG4uHoUx0ePj/2qrnTnOBBNE4MvgDuTJzy+caU
k8RqmdHCbHzTe6fzYq/9it8sZ77KVN1qkbIcuc+RTxA9nNh1TjsRne74Z0j1FCLk
hHcqH0ri7PYSKHTE8FvFCxZYdbuB84CmZihvxbpRAoGAIbjqaMYPTYuklCda5S79
YSFJ1JzZe1Kja//tDw1zFcgVCKa31jAwciz0f/lSRq3HS1GGGmezhPVTiqLfeZqc
R0iKbhgbOcVVkJJ3K0yAyKwPTumxKHZ6zImZS0c0am+RY9YGq5T7YrzpzcfvpiOU
ffe3RyFT7cfCmfoOhDCtzukCgYB30oLC1RLFOrqn43vCS51zc5zoY44uBzspwwYN
TwvP/ExWMf3VJrDjBCH+T/6sysePbJEImlzM+IwytFpANfiIXEt/48Xf60Nx8gWM
uHyxZZx/NKtDw0V8vX1POnq2A5eiKa+8jRARYKJLYNdfDuwolxvG6bZhkPi/4EtT
3Y18sQKBgHtKbk+7lNJVeswXE5cUG6EDUsDe/2Ua7fXp7FcjqBEoap1LSw+6TXp0
ZgrmKE8ARzM47+EJHUviiq/nupE15g0kJW3syhpU9zZLO7ltB0KIkO9ZRcmUjo8Q
cpLlHMAqbLJ8WYGJCkhiWxyal6hYTyWY4cVkC0xtTl/hUE9IeNKo
-----END RSA PRIVATE KEY-----`)
)

var _ = Describe("ChangeProcessor", func() {
	// graph outputs are large, so allow gomega to print everything on test failure
	format.MaxLength = 0
	Describe("Normal cases of processing changes", func() {
		var (
			gc = &v1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:       gcName,
					Generation: 1,
				},
				Spec: v1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			}
			processor state.ChangeProcessor
		)

		BeforeEach(OncePerOrdered, func() {
			processor = state.NewChangeProcessorImpl(state.ChangeProcessorConfig{
				GatewayCtlrName:  controllerName,
				GatewayClassName: gcName,
				Logger:           zap.New(),
				Validators:       createAlwaysValidValidators(),
				Scheme:           createScheme(),
			})
		})

		Describe("Process gateway resources", Ordered, func() {
			var (
				gcUpdated                           *v1.GatewayClass
				diffNsTLSSecret, sameNsTLSSecret    *apiv1.Secret
				hr1, hr1Updated, hr2                *v1.HTTPRoute
				gw1, gw1Updated, gw2                *v1.Gateway
				refGrant1, refGrant2                *v1beta1.ReferenceGrant
				expGraph                            *graph.Graph
				expRouteHR1, expRouteHR2            *graph.Route
				hr1Name, hr2Name                    types.NamespacedName
				gatewayAPICRD, gatewayAPICRDUpdated *metav1.PartialObjectMetadata
			)
			BeforeAll(func() {
				gcUpdated = gc.DeepCopy()
				gcUpdated.Generation++

				crossNsBackendRef := v1.HTTPBackendRef{
					BackendRef: v1.BackendRef{
						BackendObjectReference: v1.BackendObjectReference{
							Kind:      helpers.GetPointer[v1.Kind]("Service"),
							Name:      "service",
							Namespace: helpers.GetPointer[v1.Namespace]("service-ns"),
							Port:      helpers.GetPointer[v1.PortNumber](80),
						},
					},
				}

				hr1 = createRoute("hr-1", "gateway-1", "foo.example.com", crossNsBackendRef)
				hr1Name = types.NamespacedName{Namespace: hr1.Namespace, Name: hr1.Name}

				hr1Updated = hr1.DeepCopy()
				hr1Updated.Generation++

				hr2 = createRoute("hr-2", "gateway-2", "bar.example.com")
				hr2Name = types.NamespacedName{Namespace: "test", Name: "hr-2"}

				refGrant1 = &v1beta1.ReferenceGrant{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "cert-ns",
						Name:      "ref-grant",
					},
					Spec: v1beta1.ReferenceGrantSpec{
						From: []v1beta1.ReferenceGrantFrom{
							{
								Group:     v1.GroupName,
								Kind:      "Gateway",
								Namespace: "test",
							},
						},
						To: []v1beta1.ReferenceGrantTo{
							{
								Kind: "Secret",
							},
						},
					},
				}

				refGrant2 = &v1beta1.ReferenceGrant{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "service-ns",
						Name:      "ref-grant",
					},
					Spec: v1beta1.ReferenceGrantSpec{
						From: []v1beta1.ReferenceGrantFrom{
							{
								Group:     v1.GroupName,
								Kind:      "HTTPRoute",
								Namespace: "test",
							},
						},
						To: []v1beta1.ReferenceGrantTo{
							{
								Kind: "Service",
							},
						},
					},
				}

				sameNsTLSSecret = &apiv1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tls-secret",
						Namespace: "test",
					},
					Type: apiv1.SecretTypeTLS,
					Data: map[string][]byte{
						apiv1.TLSCertKey:       cert,
						apiv1.TLSPrivateKeyKey: key,
					},
				}

				diffNsTLSSecret = &apiv1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "different-ns-tls-secret",
						Namespace: "cert-ns",
					},
					Type: apiv1.SecretTypeTLS,
					Data: map[string][]byte{
						apiv1.TLSCertKey:       cert,
						apiv1.TLSPrivateKeyKey: key,
					},
				}

				gw1 = createGatewayWithTLSListener("gateway-1", diffNsTLSSecret) // cert in diff namespace than gw

				gw1Updated = gw1.DeepCopy()
				gw1Updated.Generation++

				gw2 = createGatewayWithTLSListener("gateway-2", sameNsTLSSecret)

				gatewayAPICRD = &metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CustomResourceDefinition",
						APIVersion: "apiextensions.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "gatewayclasses.gateway.networking.k8s.io",
						Annotations: map[string]string{
							gatewayclass.BundleVersionAnnotation: gatewayclass.SupportedVersion,
						},
					},
				}

				gatewayAPICRDUpdated = gatewayAPICRD.DeepCopy()
				gatewayAPICRDUpdated.Annotations[gatewayclass.BundleVersionAnnotation] = "v1.99.0"
			})
			BeforeEach(func() {
				expRouteHR1 = &graph.Route{
					Source: hr1,
					ParentRefs: []graph.ParentRef{
						{
							Attachment: &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{"listener-80-1": {"foo.example.com"}},
								Attached:          true,
							},
							Gateway: types.NamespacedName{Namespace: "test", Name: "gateway-1"},
						},
						{
							Attachment: &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{"listener-443-1": {"foo.example.com"}},
								Attached:          true,
							},
							Gateway: types.NamespacedName{Namespace: "test", Name: "gateway-1"},
							Idx:     1,
						},
					},
					Rules: []graph.Rule{
						{
							BackendRefs: []graph.BackendRef{
								{
									SvcNsName: types.NamespacedName{Namespace: "service-ns", Name: "service"},
									Weight:    1,
								},
							},
							ValidMatches: true,
							ValidFilters: true,
						},
					},
					Valid:      true,
					Attachable: true,
					Conditions: []conditions.Condition{
						staticConds.NewRouteBackendRefRefBackendNotFound(
							"spec.rules[0].backendRefs[0].name: Not found: \"service\"",
						),
					},
				}

				expRouteHR2 = &graph.Route{
					Source: hr2,
					ParentRefs: []graph.ParentRef{
						{
							Attachment: &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{"listener-80-1": {"bar.example.com"}},
								Attached:          true,
							},
							Gateway: types.NamespacedName{Namespace: "test", Name: "gateway-2"},
						},
						{
							Attachment: &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{"listener-443-1": {"bar.example.com"}},
								Attached:          true,
							},
							Gateway: types.NamespacedName{Namespace: "test", Name: "gateway-2"},
							Idx:     1,
						},
					},
					Rules:      []graph.Rule{{ValidMatches: true, ValidFilters: true}},
					Valid:      true,
					Attachable: true,
				}

				// This is the base case expected graph. Tests will manipulate this to add or remove elements
				// to fit the expected output of the input under test.
				expGraph = &graph.Graph{
					GatewayClass: &graph.GatewayClass{
						Source: gc,
						Valid:  true,
					},
					Gateway: &graph.Gateway{
						Source: gw1,
						Listeners: []*graph.Listener{
							{
								Name:       "listener-80-1",
								Source:     gw1.Spec.Listeners[0],
								Valid:      true,
								Attachable: true,
								Routes: map[types.NamespacedName]*graph.Route{
									{Namespace: "test", Name: "hr-1"}: expRouteHR1,
								},
								SupportedKinds: []v1.RouteGroupKind{{Kind: "HTTPRoute"}},
							},
							{
								Name:       "listener-443-1",
								Source:     gw1.Spec.Listeners[1],
								Valid:      true,
								Attachable: true,
								Routes: map[types.NamespacedName]*graph.Route{
									{Namespace: "test", Name: "hr-1"}: expRouteHR1,
								},
								ResolvedSecret: helpers.GetPointer(client.ObjectKeyFromObject(diffNsTLSSecret)),
								SupportedKinds: []v1.RouteGroupKind{{Kind: "HTTPRoute"}},
							},
						},
						Valid: true,
					},
					IgnoredGateways: map[types.NamespacedName]*v1.Gateway{},
					Routes: map[types.NamespacedName]*graph.Route{
						{Namespace: "test", Name: "hr-1"}: expRouteHR1,
					},
					ReferencedSecrets: map[types.NamespacedName]*graph.Secret{},
					ReferencedServices: map[types.NamespacedName]struct{}{
						{
							Namespace: "service-ns",
							Name:      "service",
						}: {},
					},
				}
			})
			When("no upsert has occurred", func() {
				It("returns nil graph", func() {
					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
					Expect(graphCfg).To(BeNil())
					Expect(processor.GetLatestGraph()).To(BeNil())
				})
			})
			When("GatewayClass doesn't exist", func() {
				When("Gateway API CRD is added", func() {
					It("returns empty graph", func() {
						processor.CaptureUpsertChange(gatewayAPICRD)

						changed, graphCfg := processor.Process()
						Expect(changed).To(Equal(state.ClusterStateChange))
						Expect(helpers.Diff(&graph.Graph{}, graphCfg)).To(BeEmpty())
						Expect(helpers.Diff(&graph.Graph{}, processor.GetLatestGraph())).To(BeEmpty())
					})
				})
				When("Gateways don't exist", func() {
					When("the first HTTPRoute is upserted", func() {
						It("returns empty graph", func() {
							processor.CaptureUpsertChange(hr1)

							changed, graphCfg := processor.Process()
							Expect(changed).To(Equal(state.ClusterStateChange))
							Expect(helpers.Diff(&graph.Graph{}, graphCfg)).To(BeEmpty())
							Expect(helpers.Diff(&graph.Graph{}, processor.GetLatestGraph())).To(BeEmpty())
						})
					})
					When("the different namespace TLS Secret is upserted", func() {
						It("returns nil graph", func() {
							processor.CaptureUpsertChange(diffNsTLSSecret)

							changed, graphCfg := processor.Process()
							Expect(changed).To(Equal(state.NoChange))
							Expect(graphCfg).To(BeNil())
							Expect(helpers.Diff(&graph.Graph{}, processor.GetLatestGraph())).To(BeEmpty())
						})
					})
					When("the first Gateway is upserted", func() {
						It("returns populated graph", func() {
							processor.CaptureUpsertChange(gw1)

							expGraph.GatewayClass = nil

							expGraph.Gateway.Conditions = staticConds.NewGatewayInvalid("GatewayClass doesn't exist")
							expGraph.Gateway.Valid = false
							expGraph.Gateway.Listeners = nil

							// no ref grant exists yet for hr1
							expGraph.Routes[hr1Name].Conditions = []conditions.Condition{
								staticConds.NewRouteBackendRefRefNotPermitted(
									"Backend ref to Service service-ns/service not permitted by any ReferenceGrant",
								),
							}
							expGraph.Routes[hr1Name].ParentRefs[0].Attachment = &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{},
								FailedCondition:   staticConds.NewRouteNoMatchingParent(),
							}
							expGraph.Routes[hr1Name].ParentRefs[1].Attachment = &graph.ParentRefAttachmentStatus{
								AcceptedHostnames: map[string][]string{},
								FailedCondition:   staticConds.NewRouteNoMatchingParent(),
							}

							expGraph.ReferencedSecrets = nil
							expGraph.ReferencedServices = nil

							expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}

							changed, graphCfg := processor.Process()
							Expect(changed).To(Equal(state.ClusterStateChange))
							Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
							Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
						})
					})
				})
			})
			When("the GatewayClass is upserted", func() {
				It("returns updated graph", func() {
					processor.CaptureUpsertChange(gc)

					// No ref grant exists yet for gw1
					// so the listener is not valid, but still attachable
					listener443 := getListenerByName(expGraph.Gateway, "listener-443-1")
					listener443.Valid = false
					listener443.ResolvedSecret = nil
					listener443.Conditions = staticConds.NewListenerRefNotPermitted(
						"Certificate ref to secret cert-ns/different-ns-tls-secret not permitted by any ReferenceGrant",
					)

					expAttachment80 := &graph.ParentRefAttachmentStatus{
						AcceptedHostnames: map[string][]string{
							"listener-80-1": {"foo.example.com"},
						},
						Attached: true,
					}

					expAttachment443 := &graph.ParentRefAttachmentStatus{
						AcceptedHostnames: map[string][]string{
							"listener-443-1": {"foo.example.com"},
						},
						Attached: true,
					}

					listener80 := getListenerByName(expGraph.Gateway, "listener-80-1")
					listener80.Routes[hr1Name].ParentRefs[0].Attachment = expAttachment80
					listener443.Routes[hr1Name].ParentRefs[1].Attachment = expAttachment443

					// no ref grant exists yet for hr1
					expGraph.Routes[hr1Name].Conditions = []conditions.Condition{
						staticConds.NewRouteInvalidListener(),
						staticConds.NewRouteBackendRefRefNotPermitted(
							"Backend ref to Service service-ns/service not permitted by any ReferenceGrant",
						),
					}
					expGraph.Routes[hr1Name].ParentRefs[0].Attachment = expAttachment80
					expGraph.Routes[hr1Name].ParentRefs[1].Attachment = expAttachment443

					expGraph.ReferencedSecrets = nil
					expGraph.ReferencedServices = nil

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the ReferenceGrant allowing the Gateway to reference its Secret is upserted", func() {
				It("returns updated graph", func() {
					processor.CaptureUpsertChange(refGrant1)

					// no ref grant exists yet for hr1
					expGraph.Routes[hr1Name].Conditions = []conditions.Condition{
						staticConds.NewRouteBackendRefRefNotPermitted(
							"Backend ref to Service service-ns/service not permitted by any ReferenceGrant",
						),
					}
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					expGraph.ReferencedServices = nil
					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the ReferenceGrant allowing the hr1 to reference the Service in different ns is upserted", func() {
				It("returns updated graph", func() {
					processor.CaptureUpsertChange(refGrant2)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the Gateway API CRD with bundle version annotation change is processed", func() {
				It("returns updated graph", func() {
					processor.CaptureUpsertChange(gatewayAPICRDUpdated)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					expGraph.GatewayClass.Conditions = conditions.NewGatewayClassSupportedVersionBestEffort(
						gatewayclass.SupportedVersion,
					)

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the Gateway API CRD without bundle version annotation change is processed", func() {
				It("returns nil graph", func() {
					gatewayAPICRDSameVersion := gatewayAPICRDUpdated.DeepCopy()

					processor.CaptureUpsertChange(gatewayAPICRDSameVersion)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					expGraph.GatewayClass.Conditions = conditions.NewGatewayClassSupportedVersionBestEffort(
						gatewayclass.SupportedVersion,
					)

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
					Expect(graphCfg).To(BeNil())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the Gateway API CRD with bundle version annotation change is processed", func() {
				It("returns updated graph", func() {
					// change back to supported version
					processor.CaptureUpsertChange(gatewayAPICRD)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the first HTTPRoute update with a generation changed is processed", func() {
				It("returns populated graph", func() {
					processor.CaptureUpsertChange(hr1Updated)

					listener443 := getListenerByName(expGraph.Gateway, "listener-443-1")
					listener443.Routes[hr1Name].Source.Generation = hr1Updated.Generation

					listener80 := getListenerByName(expGraph.Gateway, "listener-80-1")
					listener80.Routes[hr1Name].Source.Generation = hr1Updated.Generation
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				},
				)
			})
			When("the first Gateway update with a generation changed is processed", func() {
				It("returns populated graph", func() {
					processor.CaptureUpsertChange(gw1Updated)

					expGraph.Gateway.Source.Generation = gw1Updated.Generation
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the GatewayClass update with generation change is processed", func() {
				It("returns populated graph", func() {
					processor.CaptureUpsertChange(gcUpdated)

					expGraph.GatewayClass.Source.Generation = gcUpdated.Generation
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the different namespace TLS secret is upserted again", func() {
				It("returns populated graph", func() {
					processor.CaptureUpsertChange(diffNsTLSSecret)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("no changes are captured", func() {
				It("returns nil graph", func() {
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
					Expect(graphCfg).To(BeNil())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the same namespace TLS Secret is upserted", func() {
				It("returns nil graph", func() {
					processor.CaptureUpsertChange(sameNsTLSSecret)

					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
					Expect(graphCfg).To(BeNil())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the second Gateway is upserted", func() {
				It("returns populated graph using first gateway", func() {
					processor.CaptureUpsertChange(gw2)

					expGraph.IgnoredGateways = map[types.NamespacedName]*v1.Gateway{
						{Namespace: "test", Name: "gateway-2"}: gw2,
					}
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the second HTTPRoute is upserted", func() {
				It("returns populated graph", func() {
					processor.CaptureUpsertChange(hr2)

					expGraph.IgnoredGateways = map[types.NamespacedName]*v1.Gateway{
						{Namespace: "test", Name: "gateway-2"}: gw2,
					}
					expGraph.Routes[hr2Name] = expRouteHR2
					expGraph.Routes[hr2Name].ParentRefs[0].Attachment = &graph.ParentRefAttachmentStatus{
						AcceptedHostnames: map[string][]string{},
						FailedCondition:   staticConds.NewTODO("Gateway is ignored"),
					}
					expGraph.Routes[hr2Name].ParentRefs[1].Attachment = &graph.ParentRefAttachmentStatus{
						AcceptedHostnames: map[string][]string{},
						FailedCondition:   staticConds.NewTODO("Gateway is ignored"),
					}
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(diffNsTLSSecret)] = &graph.Secret{
						Source: diffNsTLSSecret,
					}

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the first Gateway is deleted", func() {
				It("returns updated graph", func() {
					processor.CaptureDeleteChange(
						&v1.Gateway{},
						types.NamespacedName{Namespace: "test", Name: "gateway-1"},
					)

					// gateway 2 takes over;
					// route 1 has been replaced by route 2
					listener80 := getListenerByName(expGraph.Gateway, "listener-80-1")
					listener443 := getListenerByName(expGraph.Gateway, "listener-443-1")

					expGraph.Gateway.Source = gw2
					listener80.Source = gw2.Spec.Listeners[0]
					listener443.Source = gw2.Spec.Listeners[1]
					delete(listener80.Routes, hr1Name)
					delete(listener443.Routes, hr1Name)
					listener80.Routes[hr2Name] = expRouteHR2
					listener443.Routes[hr2Name] = expRouteHR2
					delete(expGraph.Routes, hr1Name)
					expGraph.Routes[hr2Name] = expRouteHR2
					sameNsTLSSecretRef := helpers.GetPointer(client.ObjectKeyFromObject(sameNsTLSSecret))
					listener443.ResolvedSecret = sameNsTLSSecretRef
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(sameNsTLSSecret)] = &graph.Secret{
						Source: sameNsTLSSecret,
					}

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}
					expGraph.ReferencedServices = nil

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the second HTTPRoute is deleted", func() {
				It("returns updated graph", func() {
					processor.CaptureDeleteChange(
						&v1.HTTPRoute{},
						types.NamespacedName{Namespace: "test", Name: "hr-2"},
					)

					// gateway 2 still in charge;
					// no routes remain
					listener80 := getListenerByName(expGraph.Gateway, "listener-80-1")
					listener443 := getListenerByName(expGraph.Gateway, "listener-443-1")

					expGraph.Gateway.Source = gw2
					listener80.Source = gw2.Spec.Listeners[0]
					listener443.Source = gw2.Spec.Listeners[1]
					delete(listener80.Routes, hr1Name)
					delete(listener443.Routes, hr1Name)
					expGraph.Routes = map[types.NamespacedName]*graph.Route{}
					sameNsTLSSecretRef := helpers.GetPointer(client.ObjectKeyFromObject(sameNsTLSSecret))
					listener443.ResolvedSecret = sameNsTLSSecretRef
					expGraph.ReferencedSecrets[client.ObjectKeyFromObject(sameNsTLSSecret)] = &graph.Secret{
						Source: sameNsTLSSecret,
					}

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}
					expGraph.ReferencedServices = nil

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the GatewayClass is deleted", func() {
				It("returns updated graph", func() {
					processor.CaptureDeleteChange(
						&v1.GatewayClass{},
						types.NamespacedName{Name: gcName},
					)

					expGraph.GatewayClass = nil
					expGraph.Gateway = &graph.Gateway{
						Source:     gw2,
						Conditions: staticConds.NewGatewayInvalid("GatewayClass doesn't exist"),
					}
					expGraph.Routes = map[types.NamespacedName]*graph.Route{}
					expGraph.ReferencedSecrets = nil

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}
					expGraph.ReferencedServices = nil

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(expGraph, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(expGraph, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the second Gateway is deleted", func() {
				It("returns empty graph", func() {
					processor.CaptureDeleteChange(
						&v1.Gateway{},
						types.NamespacedName{Namespace: "test", Name: "gateway-2"},
					)

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}
					expGraph.ReferencedServices = nil

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(&graph.Graph{}, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(&graph.Graph{}, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
			When("the first HTTPRoute is deleted", func() {
				It("returns empty graph", func() {
					processor.CaptureDeleteChange(
						&v1.HTTPRoute{},
						types.NamespacedName{Namespace: "test", Name: "hr-1"},
					)

					expRouteHR1.Rules[0].BackendRefs[0].SvcNsName = types.NamespacedName{}
					expGraph.ReferencedServices = nil

					changed, graphCfg := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
					Expect(helpers.Diff(&graph.Graph{}, graphCfg)).To(BeEmpty())
					Expect(helpers.Diff(&graph.Graph{}, processor.GetLatestGraph())).To(BeEmpty())
				})
			})
		})

		Describe("Process services and endpoints", Ordered, func() {
			var (
				hr1, hr2, hr3, hrInvalidBackendRef, hrMultipleRules                 *v1.HTTPRoute
				hr1svc, sharedSvc, bazSvc1, bazSvc2, bazSvc3, invalidSvc, notRefSvc *apiv1.Service
				hr1slice1, hr1slice2, noRefSlice, missingSvcNameSlice               *discoveryV1.EndpointSlice
				gw                                                                  *v1.Gateway
				btls                                                                *v1alpha2.BackendTLSPolicy
			)

			createSvc := func(name string) *apiv1.Service {
				return &apiv1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      name,
					},
				}
			}

			createEndpointSlice := func(name string, svcName string) *discoveryV1.EndpointSlice {
				return &discoveryV1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      name,
						Labels:    map[string]string{index.KubernetesServiceNameLabel: svcName},
					},
				}
			}

			createBackendTLSPolicy := func(name string, svcName string) *v1alpha2.BackendTLSPolicy {
				return &v1alpha2.BackendTLSPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
						Name:      name,
					},
					Spec: v1alpha2.BackendTLSPolicySpec{
						TargetRef: v1alpha2.PolicyTargetReferenceWithSectionName{
							PolicyTargetReference: v1alpha2.PolicyTargetReference{
								Kind:      v1.Kind("Service"),
								Name:      v1.ObjectName(svcName),
								Namespace: helpers.GetPointer(v1.Namespace("test")),
							},
						},
					},
				}
			}

			BeforeAll(func() {
				testNamespace := v1.Namespace("test")
				kindService := v1.Kind("Service")
				kindInvalid := v1.Kind("Invalid")

				// backend Refs
				fooRef := createBackendRef(&kindService, "foo-svc", &testNamespace)
				baz1NilNamespace := createBackendRef(&kindService, "baz-svc-v1", &testNamespace)
				barRef := createBackendRef(&kindService, "bar-svc", nil)
				baz2Ref := createBackendRef(&kindService, "baz-svc-v2", &testNamespace)
				baz3Ref := createBackendRef(&kindService, "baz-svc-v3", &testNamespace)
				invalidKindRef := createBackendRef(&kindInvalid, "bar-svc", &testNamespace)

				// httproutes
				hr1 = createRoute("hr1", "gw", "foo.example.com", fooRef)
				hr2 = createRoute("hr2", "gw", "bar.example.com", barRef)
				// hr3 shares the same backendRef as hr2
				hr3 = createRoute("hr3", "gw", "bar.2.example.com", barRef)
				hrInvalidBackendRef = createRoute("hr-invalid", "gw", "invalid.com", invalidKindRef)
				hrMultipleRules = createRouteWithMultipleRules(
					"hr-multiple-rules",
					"gw",
					"mutli.example.com",
					[]v1.HTTPRouteRule{
						createHTTPRule("/baz-v1", baz1NilNamespace),
						createHTTPRule("/baz-v2", baz2Ref),
						createHTTPRule("/baz-v3", baz3Ref),
					},
				)

				// services
				hr1svc = createSvc("foo-svc")
				sharedSvc = createSvc("bar-svc")  // shared between hr2 and hr3
				invalidSvc = createSvc("invalid") // nsname matches invalid BackendRef
				notRefSvc = createSvc("not-ref")
				bazSvc1 = createSvc("baz-svc-v1")
				bazSvc2 = createSvc("baz-svc-v2")
				bazSvc3 = createSvc("baz-svc-v3")

				// endpoint slices
				hr1slice1 = createEndpointSlice("hr1-1", "foo-svc")
				hr1slice2 = createEndpointSlice("hr1-2", "foo-svc")
				noRefSlice = createEndpointSlice("no-ref", "no-ref")
				missingSvcNameSlice = createEndpointSlice("missing-svc-name", "")

				// backendTLSPolicy
				btls = createBackendTLSPolicy("btls", "foo-svc")

				gw = createGateway("gw")
				processor.CaptureUpsertChange(gc)
				processor.CaptureUpsertChange(gw)
				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})

			testProcessChangedVal := func(expChanged state.ChangeType) {
				changed, _ := processor.Process()
				Expect(changed).To(Equal(expChanged))
			}

			testUpsertTriggersChange := func(obj client.Object, expChanged state.ChangeType) {
				processor.CaptureUpsertChange(obj)
				testProcessChangedVal(expChanged)
			}

			testDeleteTriggersChange := func(obj client.Object, nsname types.NamespacedName, expChanged state.ChangeType) {
				processor.CaptureDeleteChange(obj, nsname)
				testProcessChangedVal(expChanged)
			}

			When("hr1 is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1, state.ClusterStateChange)
				})
			})
			When("a hr1 service is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1svc, state.ClusterStateChange)
				})
			})
			When("a backendTLSPolicy is added for referenced service", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(btls, state.ClusterStateChange)
				})
			})
			When("an hr1 endpoint slice is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1slice1, state.EndpointsOnlyChange)
				})
			})
			When("an hr1 service is updated", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1svc, state.ClusterStateChange)
				})
			})
			When("another hr1 endpoint slice is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1slice2, state.EndpointsOnlyChange)
				})
			})
			When("an endpoint slice with a missing svc name label is added", func() {
				It("should not trigger a change", func() {
					testUpsertTriggersChange(missingSvcNameSlice, state.NoChange)
				})
			})
			When("an hr1 endpoint slice is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						hr1slice1,
						types.NamespacedName{Namespace: hr1slice1.Namespace, Name: hr1slice1.Name},
						state.EndpointsOnlyChange,
					)
				})
			})
			When("the second hr1 endpoint slice is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						hr1slice2,
						types.NamespacedName{Namespace: hr1slice2.Namespace, Name: hr1slice2.Name},
						state.EndpointsOnlyChange,
					)
				})
			})
			When("the second hr1 endpoint slice is recreated", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr1slice2, state.EndpointsOnlyChange)
				})
			})
			When("hr1 is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						hr1,
						types.NamespacedName{Namespace: hr1.Namespace, Name: hr1.Name},
						state.ClusterStateChange,
					)
				})
			})
			When("hr1 service is deleted", func() {
				It("should not trigger a change", func() {
					testDeleteTriggersChange(
						hr1svc,
						types.NamespacedName{Namespace: hr1svc.Namespace, Name: hr1svc.Name},
						state.NoChange,
					)
				})
			})
			When("the second hr1 endpoint slice is deleted", func() {
				It("should not trigger a change", func() {
					testDeleteTriggersChange(
						hr1slice2,
						types.NamespacedName{Namespace: hr1slice2.Namespace, Name: hr1slice2.Name},
						state.NoChange,
					)
				})
			})
			When("hr2 is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr2, state.ClusterStateChange)
				})
			})
			When("a hr3, that shares a backend service with hr2, is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hr3, state.ClusterStateChange)
				})
			})
			When("sharedSvc, a service referenced by both hr2 and hr3, is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(sharedSvc, state.ClusterStateChange)
				})
			})
			When("hr2 is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						hr2,
						types.NamespacedName{Namespace: hr2.Namespace, Name: hr2.Name},
						state.ClusterStateChange,
					)
				})
			})
			When("sharedSvc is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						sharedSvc,
						types.NamespacedName{Namespace: sharedSvc.Namespace, Name: sharedSvc.Name},
						state.ClusterStateChange,
					)
				})
			})
			When("sharedSvc is recreated", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(sharedSvc, state.ClusterStateChange)
				})
			})
			When("hr3 is deleted", func() {
				It("should trigger a change", func() {
					testDeleteTriggersChange(
						hr3,
						types.NamespacedName{Namespace: hr3.Namespace, Name: hr3.Name},
						state.ClusterStateChange,
					)
				})
			})
			When("sharedSvc is deleted", func() {
				It("should not trigger a change", func() {
					testDeleteTriggersChange(
						sharedSvc,
						types.NamespacedName{Namespace: sharedSvc.Namespace, Name: sharedSvc.Name},
						state.NoChange,
					)
				})
			})
			When("a service that is not referenced by any route is added", func() {
				It("should not trigger a change", func() {
					testUpsertTriggersChange(notRefSvc, state.NoChange)
				})
			})
			When("a route with an invalid backend ref type is added", func() {
				It("should trigger a change", func() {
					testUpsertTriggersChange(hrInvalidBackendRef, state.ClusterStateChange)
				})
			})
			When("a service with a namespace name that matches invalid backend ref is added", func() {
				It("should not trigger a change", func() {
					testUpsertTriggersChange(invalidSvc, state.NoChange)
				})
			})
			When("an endpoint slice that is not owned by a referenced service is added", func() {
				It("should not trigger a change", func() {
					testUpsertTriggersChange(noRefSlice, state.NoChange)
				})
			})
			When("an endpoint slice that is not owned by a referenced service is deleted", func() {
				It("should not trigger a change", func() {
					testDeleteTriggersChange(
						noRefSlice,
						types.NamespacedName{Namespace: noRefSlice.Namespace, Name: noRefSlice.Name},
						state.NoChange,
					)
				})
			})
			Context("processing a route with multiple rules and three unique backend services", func() {
				When("route is added", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(hrMultipleRules, state.ClusterStateChange)
					})
				})
				When("first referenced service is added", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(bazSvc1, state.ClusterStateChange)
					})
				})
				When("second referenced service is added", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(bazSvc2, state.ClusterStateChange)
					})
				})
				When("first referenced service is deleted", func() {
					It("should trigger a change", func() {
						testDeleteTriggersChange(
							bazSvc1,
							types.NamespacedName{Namespace: bazSvc1.Namespace, Name: bazSvc1.Name},
							state.ClusterStateChange,
						)
					})
				})
				When("first referenced service is recreated", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(bazSvc1, state.ClusterStateChange)
					})
				})
				When("third referenced service is added", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(bazSvc3, state.ClusterStateChange)
					})
				})
				When("third referenced service is updated", func() {
					It("should trigger a change", func() {
						testUpsertTriggersChange(bazSvc3, state.ClusterStateChange)
					})
				})
				When("route is deleted", func() {
					It("should trigger a change", func() {
						testDeleteTriggersChange(
							hrMultipleRules,
							types.NamespacedName{
								Namespace: hrMultipleRules.Namespace,
								Name:      hrMultipleRules.Name,
							},
							state.ClusterStateChange,
						)
					})
				})
				When("first referenced service is deleted", func() {
					It("should not trigger a change", func() {
						testDeleteTriggersChange(
							bazSvc1,
							types.NamespacedName{Namespace: bazSvc1.Namespace, Name: bazSvc1.Name},
							state.NoChange,
						)
					})
				})
				When("second referenced service is deleted", func() {
					It("should not trigger a change", func() {
						testDeleteTriggersChange(
							bazSvc2,
							types.NamespacedName{Namespace: bazSvc2.Namespace, Name: bazSvc2.Name},
							state.NoChange,
						)
					})
				})
				When("final referenced service is deleted", func() {
					It("should not trigger a change", func() {
						testDeleteTriggersChange(
							bazSvc3,
							types.NamespacedName{Namespace: bazSvc3.Namespace, Name: bazSvc3.Name},
							state.NoChange,
						)
					})
				})
			})
		})

		Describe("namespace changes", Ordered, func() {
			var (
				ns, nsDifferentLabels, nsNoLabels *apiv1.Namespace
				gw                                *v1.Gateway
			)

			BeforeAll(func() {
				ns = &apiv1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns",
						Labels: map[string]string{
							"app": "allowed",
						},
					},
				}
				nsDifferentLabels = &apiv1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-different-labels",
						Labels: map[string]string{
							"oranges": "bananas",
						},
					},
				}
				nsNoLabels = &apiv1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "no-labels",
					},
				}
				gw = &v1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw",
					},
					Spec: v1.GatewaySpec{
						GatewayClassName: gcName,
						Listeners: []v1.Listener{
							{
								Port:     80,
								Protocol: v1.HTTPProtocolType,
								AllowedRoutes: &v1.AllowedRoutes{
									Namespaces: &v1.RouteNamespaces{
										From: helpers.GetPointer(v1.NamespacesFromSelector),
										Selector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "allowed",
											},
										},
									},
								},
							},
						},
					},
				}
				processor = state.NewChangeProcessorImpl(state.ChangeProcessorConfig{
					GatewayCtlrName:  controllerName,
					GatewayClassName: gcName,
					Logger:           zap.New(),
					Validators:       createAlwaysValidValidators(),
					Scheme:           createScheme(),
				})
				processor.CaptureUpsertChange(gc)
				processor.CaptureUpsertChange(gw)
				processor.Process()
			})

			When("a namespace is created that is not linked to a listener", func() {
				It("does not trigger an update", func() {
					processor.CaptureUpsertChange(nsNoLabels)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
				})
			})
			When("a namespace is created that is linked to a listener", func() {
				It("triggers an update", func() {
					processor.CaptureUpsertChange(ns)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("a namespace is deleted that is not linked to a listener", func() {
				It("does not trigger an update", func() {
					processor.CaptureDeleteChange(nsNoLabels, types.NamespacedName{Name: "no-labels"})
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
				})
			})
			When("a namespace is deleted that is linked to a listener", func() {
				It("triggers an update", func() {
					processor.CaptureDeleteChange(ns, types.NamespacedName{Name: "ns"})
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("a namespace that is not linked to a listener has its labels changed to match a listener", func() {
				It("triggers an update", func() {
					processor.CaptureUpsertChange(nsDifferentLabels)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.NoChange))

					nsDifferentLabels.Labels = map[string]string{
						"app": "allowed",
					}
					processor.CaptureUpsertChange(nsDifferentLabels)
					changed, _ = processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("a namespace that is linked to a listener has its labels changed to no longer match a listener", func() {
				It("triggers an update", func() {
					nsDifferentLabels.Labels = map[string]string{
						"oranges": "bananas",
					}
					processor.CaptureUpsertChange(nsDifferentLabels)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("a gateway changes its listener's labels", func() {
				It("triggers an update when a namespace that matches the new labels is created", func() {
					gwChangedLabel := gw.DeepCopy()
					gwChangedLabel.Spec.Listeners[0].AllowedRoutes.Namespaces.Selector.MatchLabels = map[string]string{
						"oranges": "bananas",
					}
					gwChangedLabel.Generation++
					processor.CaptureUpsertChange(gwChangedLabel)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))

					// After changing the gateway's labels and generation, the processor should be marked to update
					// the nginx configuration and build a new graph. When processor.Process() gets called,
					// the nginx configuration gets updated and a new graph is built with an updated
					// referencedNamespaces. Thus, when the namespace "ns" is upserted with labels that no longer match
					// the new labels on the gateway, it would not trigger a change as the namespace would no longer
					// be in the updated referencedNamespaces and the labels no longer match the new labels on the
					// gateway.
					processor.CaptureUpsertChange(ns)
					changed, _ = processor.Process()
					Expect(changed).To(Equal(state.NoChange))

					processor.CaptureUpsertChange(nsDifferentLabels)
					changed, _ = processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("a namespace that is not linked to a listener has its labels removed", func() {
				It("does not trigger an update", func() {
					ns.Labels = nil
					processor.CaptureUpsertChange(ns)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.NoChange))
				})
			})
			When("a namespace that is linked to a listener has its labels removed", func() {
				It("triggers an update when labels are removed", func() {
					nsDifferentLabels.Labels = nil
					processor.CaptureUpsertChange(nsDifferentLabels)
					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
		})
	})

	Describe("Ensuring non-changing changes don't override previously changing changes", func() {
		// Note: in these tests, we deliberately don't fully inspect the returned configuration and statuses
		// -- this is done in 'Normal cases of processing changes'

		//nolint:lll
		var (
			processor                                                                                                     *state.ChangeProcessorImpl
			gcNsName, gwNsName, hrNsName, hr2NsName, rgNsName, svcNsName, sliceNsName, secretNsName, cmNsName, btlsNsName types.NamespacedName
			gc, gcUpdated                                                                                                 *v1.GatewayClass
			gw1, gw1Updated, gw2                                                                                          *v1.Gateway
			hr1, hr1Updated, hr2                                                                                          *v1.HTTPRoute
			rg1, rg1Updated, rg2                                                                                          *v1beta1.ReferenceGrant
			svc, barSvc, unrelatedSvc                                                                                     *apiv1.Service
			slice, barSlice, unrelatedSlice                                                                               *discoveryV1.EndpointSlice
			ns, unrelatedNS, testNs, barNs                                                                                *apiv1.Namespace
			secret, secretUpdated, unrelatedSecret, barSecret, barSecretUpdated                                           *apiv1.Secret
			cm, cmUpdated, unrelatedCM                                                                                    *apiv1.ConfigMap
			btls, btlsUpdated                                                                                             *v1alpha2.BackendTLSPolicy
		)

		BeforeEach(OncePerOrdered, func() {
			processor = state.NewChangeProcessorImpl(state.ChangeProcessorConfig{
				GatewayCtlrName:  "test.controller",
				GatewayClassName: "test-class",
				Validators:       createAlwaysValidValidators(),
				Scheme:           createScheme(),
			})

			secretNsName = types.NamespacedName{Namespace: "test", Name: "tls-secret"}
			secret = &apiv1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       secretNsName.Name,
					Namespace:  secretNsName.Namespace,
					Generation: 1,
				},
				Type: apiv1.SecretTypeTLS,
				Data: map[string][]byte{
					apiv1.TLSCertKey:       cert,
					apiv1.TLSPrivateKeyKey: key,
				},
			}
			secretUpdated = secret.DeepCopy()
			secretUpdated.Generation++
			barSecret = &apiv1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "bar-secret",
					Namespace:  "test",
					Generation: 1,
				},
				Type: apiv1.SecretTypeTLS,
				Data: map[string][]byte{
					apiv1.TLSCertKey:       cert,
					apiv1.TLSPrivateKeyKey: key,
				},
			}
			barSecretUpdated = barSecret.DeepCopy()
			barSecretUpdated.Generation++
			unrelatedSecret = &apiv1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "unrelated-tls-secret",
					Namespace:  "unrelated-ns",
					Generation: 1,
				},
				Type: apiv1.SecretTypeTLS,
				Data: map[string][]byte{
					apiv1.TLSCertKey:       cert,
					apiv1.TLSPrivateKeyKey: key,
				},
			}

			gcNsName = types.NamespacedName{Name: "test-class"}

			gc = &v1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gcNsName.Name,
				},
				Spec: v1.GatewayClassSpec{
					ControllerName: "test.controller",
				},
			}

			gcUpdated = gc.DeepCopy()
			gcUpdated.Generation++

			gwNsName = types.NamespacedName{Namespace: "test", Name: "gw-1"}

			gw1 = &v1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gw-1",
					Namespace:  "test",
					Generation: 1,
				},
				Spec: v1.GatewaySpec{
					GatewayClassName: gcName,
					Listeners: []v1.Listener{
						{
							Name:     "listener-80-1",
							Hostname: nil,
							Port:     80,
							Protocol: v1.HTTPProtocolType,
							AllowedRoutes: &v1.AllowedRoutes{
								Namespaces: &v1.RouteNamespaces{
									From: helpers.GetPointer(v1.NamespacesFromSelector),
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"test": "namespace",
										},
									},
								},
							},
						},
						{
							Name:     "listener-443-1",
							Hostname: nil,
							Port:     443,
							Protocol: v1.HTTPSProtocolType,
							TLS: &v1.GatewayTLSConfig{
								Mode: helpers.GetPointer(v1.TLSModeTerminate),
								CertificateRefs: []v1.SecretObjectReference{
									{
										Kind:      (*v1.Kind)(helpers.GetPointer("Secret")),
										Name:      v1.ObjectName(secret.Name),
										Namespace: (*v1.Namespace)(&secret.Namespace),
									},
								},
							},
						},
						{
							Name:     "listener-500-1",
							Hostname: nil,
							Port:     500,
							Protocol: v1.HTTPSProtocolType,
							TLS: &v1.GatewayTLSConfig{
								Mode: helpers.GetPointer(v1.TLSModeTerminate),
								CertificateRefs: []v1.SecretObjectReference{
									{
										Kind:      (*v1.Kind)(helpers.GetPointer("Secret")),
										Name:      v1.ObjectName(barSecret.Name),
										Namespace: (*v1.Namespace)(&barSecret.Namespace),
									},
								},
							},
						},
					},
				},
			}

			gw1Updated = gw1.DeepCopy()
			gw1Updated.Generation++

			gw2 = gw1.DeepCopy()
			gw2.Name = "gw-2"

			testNamespace := v1.Namespace("test")
			kindService := v1.Kind("Service")
			fooRef := createBackendRef(&kindService, "foo-svc", &testNamespace)
			barRef := createBackendRef(&kindService, "bar-svc", &testNamespace)

			hrNsName = types.NamespacedName{Namespace: "test", Name: "hr-1"}

			hr1 = createRoute("hr-1", "gw-1", "foo.example.com", fooRef, barRef)

			hr1Updated = hr1.DeepCopy()
			hr1Updated.Generation++

			hr2NsName = types.NamespacedName{Namespace: "test", Name: "hr-2"}

			hr2 = hr1.DeepCopy()
			hr2.Name = hr2NsName.Name

			svcNsName = types.NamespacedName{Namespace: "test", Name: "foo-svc"}
			svc = &apiv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: svcNsName.Namespace,
					Name:      svcNsName.Name,
				},
			}
			barSvc = &apiv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "bar-svc",
				},
			}
			unrelatedSvc = &apiv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "unrelated-svc",
				},
			}

			sliceNsName = types.NamespacedName{Namespace: "test", Name: "slice"}
			slice = &discoveryV1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: sliceNsName.Namespace,
					Name:      sliceNsName.Name,
					Labels:    map[string]string{index.KubernetesServiceNameLabel: svc.Name},
				},
			}
			barSlice = &discoveryV1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "bar-slice",
					Labels:    map[string]string{index.KubernetesServiceNameLabel: "bar-svc"},
				},
			}
			unrelatedSlice = &discoveryV1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "unrelated-slice",
					Labels:    map[string]string{index.KubernetesServiceNameLabel: "unrelated-svc"},
				},
			}

			testNs = &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"test": "namespace",
					},
				},
			}
			ns = &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
					Labels: map[string]string{
						"test": "namespace",
					},
				},
			}
			barNs = &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bar-ns",
					Labels: map[string]string{
						"test": "namespace",
					},
				},
			}
			unrelatedNS = &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unrelated-ns",
					Labels: map[string]string{
						"oranges": "bananas",
					},
				},
			}

			rgNsName = types.NamespacedName{Namespace: "test", Name: "rg-1"}

			rg1 = &v1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rgNsName.Name,
					Namespace: rgNsName.Namespace,
				},
			}

			rg1Updated = rg1.DeepCopy()
			rg1Updated.Generation++

			rg2 = rg1.DeepCopy()
			rg2.Name = "rg-2"

			cmNsName = types.NamespacedName{Namespace: "test", Name: "cm-1"}
			cm = &apiv1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmNsName.Name,
					Namespace: cmNsName.Namespace,
				},
				Data: map[string]string{
					"ca.crt": "value",
				},
			}
			cmUpdated = cm.DeepCopy()
			cmUpdated.Data["ca.crt"] = "updated-value"

			unrelatedCM = &apiv1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-cm",
					Namespace: "unrelated-ns",
				},
				Data: map[string]string{
					"ca.crt": "value",
				},
			}

			btlsNsName = types.NamespacedName{Namespace: "test", Name: "btls-1"}
			btls = &v1alpha2.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       btlsNsName.Name,
					Namespace:  btlsNsName.Namespace,
					Generation: 1,
				},
				Spec: v1alpha2.BackendTLSPolicySpec{
					TargetRef: v1alpha2.PolicyTargetReferenceWithSectionName{
						PolicyTargetReference: v1alpha2.PolicyTargetReference{
							Kind:      "Service",
							Name:      v1.ObjectName(svc.Name),
							Namespace: helpers.GetPointer(v1.Namespace(svc.Namespace)),
						},
					},
					TLS: v1alpha2.BackendTLSPolicyConfig{
						CACertRefs: []v1.LocalObjectReference{
							{
								Name: v1.ObjectName(cm.Name),
							},
						},
					},
				},
			}
			btlsUpdated = btls.DeepCopy()
		})
		// Changing change - a change that makes processor.Process() report changed
		// Non-changing change - a change that doesn't do that
		// Related resource - a K8s resource that is related to a configured Gateway API resource
		// Unrelated resource - a K8s resource that is not related to a configured Gateway API resource

		// Note: in these tests, we deliberately don't fully inspect the returned configuration and statuses
		// -- this is done in 'Normal cases of processing changes'
		Describe("Multiple Gateway API resource changes", Ordered, func() {
			It("should report changed after multiple Upserts", func() {
				processor.CaptureUpsertChange(gc)
				processor.CaptureUpsertChange(gw1)
				processor.CaptureUpsertChange(testNs)
				processor.CaptureUpsertChange(hr1)
				processor.CaptureUpsertChange(rg1)
				processor.CaptureUpsertChange(btls)
				processor.CaptureUpsertChange(cm)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})
			When("a upsert of updated resources is followed by an upsert of the same generation", func() {
				It("should report changed", func() {
					// these are changing changes
					processor.CaptureUpsertChange(gcUpdated)
					processor.CaptureUpsertChange(gw1Updated)
					processor.CaptureUpsertChange(hr1Updated)
					processor.CaptureUpsertChange(rg1Updated)
					processor.CaptureUpsertChange(btlsUpdated)
					processor.CaptureUpsertChange(cmUpdated)

					// there are non-changing changes
					processor.CaptureUpsertChange(gcUpdated)
					processor.CaptureUpsertChange(gw1Updated)
					processor.CaptureUpsertChange(hr1Updated)
					processor.CaptureUpsertChange(rg1Updated)
					processor.CaptureUpsertChange(btlsUpdated)
					processor.CaptureUpsertChange(cmUpdated)

					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			It("should report changed after upserting new resources", func() {
				// we can't have a second GatewayClass, so we don't add it
				processor.CaptureUpsertChange(gw2)
				processor.CaptureUpsertChange(hr2)
				processor.CaptureUpsertChange(rg2)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})
			When("resources are deleted followed by upserts with the same generations", func() {
				It("should report changed", func() {
					// these are changing changes
					processor.CaptureDeleteChange(&v1.GatewayClass{}, gcNsName)
					processor.CaptureDeleteChange(&v1.Gateway{}, gwNsName)
					processor.CaptureDeleteChange(&v1.HTTPRoute{}, hrNsName)
					processor.CaptureDeleteChange(&v1beta1.ReferenceGrant{}, rgNsName)
					processor.CaptureDeleteChange(&v1alpha2.BackendTLSPolicy{}, btlsNsName)
					processor.CaptureDeleteChange(&apiv1.ConfigMap{}, cmNsName)

					// these are non-changing changes
					processor.CaptureUpsertChange(gw2)
					processor.CaptureUpsertChange(hr2)
					processor.CaptureUpsertChange(rg2)

					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			It("should report changed after deleting resources", func() {
				processor.CaptureDeleteChange(&v1.HTTPRoute{}, hr2NsName)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})
		})
		Describe("Deleting non-existing Gateway API resource", func() {
			It("should not report changed after deleting non-existing", func() {
				processor.CaptureDeleteChange(&v1.GatewayClass{}, gcNsName)
				processor.CaptureDeleteChange(&v1.Gateway{}, gwNsName)
				processor.CaptureDeleteChange(&v1.HTTPRoute{}, hrNsName)
				processor.CaptureDeleteChange(&v1.HTTPRoute{}, hr2NsName)
				processor.CaptureDeleteChange(&v1beta1.ReferenceGrant{}, rgNsName)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.NoChange))
			})
		})
		Describe("Multiple Kubernetes API resource changes", Ordered, func() {
			BeforeAll(func() {
				// Set up graph
				processor.CaptureUpsertChange(gc)
				processor.CaptureUpsertChange(gw1)
				processor.CaptureUpsertChange(testNs)
				processor.CaptureUpsertChange(hr1)
				processor.CaptureUpsertChange(secret)
				processor.CaptureUpsertChange(barSecret)
				processor.CaptureUpsertChange(cm)
				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})

			It("should report changed after multiple Upserts of related resources", func() {
				processor.CaptureUpsertChange(svc)
				processor.CaptureUpsertChange(slice)
				processor.CaptureUpsertChange(ns)
				processor.CaptureUpsertChange(secretUpdated)
				processor.CaptureUpsertChange(cmUpdated)
				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})
			It("should report not changed after multiple Upserts of unrelated resources", func() {
				processor.CaptureUpsertChange(unrelatedSvc)
				processor.CaptureUpsertChange(unrelatedSlice)
				processor.CaptureUpsertChange(unrelatedNS)
				processor.CaptureUpsertChange(unrelatedSecret)
				processor.CaptureUpsertChange(unrelatedCM)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.NoChange))
			})
			When("upserts of related resources are followed by upserts of unrelated resources", func() {
				It("should report changed", func() {
					// these are changing changes
					processor.CaptureUpsertChange(barSvc)
					processor.CaptureUpsertChange(barSlice)
					processor.CaptureUpsertChange(barNs)
					processor.CaptureUpsertChange(barSecretUpdated)
					processor.CaptureUpsertChange(cmUpdated)

					// there are non-changing changes
					processor.CaptureUpsertChange(unrelatedSvc)
					processor.CaptureUpsertChange(unrelatedSlice)
					processor.CaptureUpsertChange(unrelatedNS)
					processor.CaptureUpsertChange(unrelatedSecret)
					processor.CaptureUpsertChange(unrelatedCM)

					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
			When("deletes of related resources are followed by upserts of unrelated resources", func() {
				It("should report changed", func() {
					// these are changing changes
					processor.CaptureDeleteChange(&apiv1.Service{}, svcNsName)
					processor.CaptureDeleteChange(&discoveryV1.EndpointSlice{}, sliceNsName)
					processor.CaptureDeleteChange(&apiv1.Namespace{}, types.NamespacedName{Name: "ns"})
					processor.CaptureDeleteChange(&apiv1.Secret{}, secretNsName)
					processor.CaptureDeleteChange(&apiv1.ConfigMap{}, cmNsName)

					// these are non-changing changes
					processor.CaptureUpsertChange(unrelatedSvc)
					processor.CaptureUpsertChange(unrelatedSlice)
					processor.CaptureUpsertChange(unrelatedNS)
					processor.CaptureUpsertChange(unrelatedSecret)
					processor.CaptureUpsertChange(unrelatedCM)

					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				})
			})
		})
		Describe("Multiple Kubernetes API and Gateway API resource changes", Ordered, func() {
			It("should report changed after multiple Upserts of new and related resources", func() {
				// new Gateway API resources
				processor.CaptureUpsertChange(gc)
				processor.CaptureUpsertChange(gw1)
				processor.CaptureUpsertChange(testNs)
				processor.CaptureUpsertChange(hr1)
				processor.CaptureUpsertChange(rg1)
				processor.CaptureUpsertChange(btls)

				// related Kubernetes API resources
				processor.CaptureUpsertChange(svc)
				processor.CaptureUpsertChange(slice)
				processor.CaptureUpsertChange(ns)
				processor.CaptureUpsertChange(secret)
				processor.CaptureUpsertChange(cm)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.ClusterStateChange))
			})
			It("should report not changed after multiple Upserts of unrelated resources", func() {
				// unrelated Kubernetes API resources
				processor.CaptureUpsertChange(unrelatedSvc)
				processor.CaptureUpsertChange(unrelatedSlice)
				processor.CaptureUpsertChange(unrelatedNS)
				processor.CaptureUpsertChange(unrelatedSecret)
				processor.CaptureUpsertChange(unrelatedCM)

				changed, _ := processor.Process()
				Expect(changed).To(Equal(state.NoChange))
			})
			It("should report changed after upserting changed resources followed by upserting unrelated resources",
				func() {
					// these are changing changes
					processor.CaptureUpsertChange(gcUpdated)
					processor.CaptureUpsertChange(gw1Updated)
					processor.CaptureUpsertChange(hr1Updated)
					processor.CaptureUpsertChange(rg1Updated)
					processor.CaptureUpsertChange(btlsUpdated)

					// these are non-changing changes
					processor.CaptureUpsertChange(unrelatedSvc)
					processor.CaptureUpsertChange(unrelatedSlice)
					processor.CaptureUpsertChange(unrelatedNS)
					processor.CaptureUpsertChange(unrelatedSecret)
					processor.CaptureUpsertChange(unrelatedCM)

					changed, _ := processor.Process()
					Expect(changed).To(Equal(state.ClusterStateChange))
				},
			)
		})
	})
	Describe("Edge cases with panic", func() {
		var processor state.ChangeProcessor

		BeforeEach(func() {
			processor = state.NewChangeProcessorImpl(state.ChangeProcessorConfig{
				GatewayCtlrName:  "test.controller",
				GatewayClassName: "my-class",
				Validators:       createAlwaysValidValidators(),
				Scheme:           createScheme(),
			})
		})

		DescribeTable("CaptureUpsertChange must panic",
			func(obj client.Object) {
				process := func() {
					processor.CaptureUpsertChange(obj)
				}
				Expect(process).Should(Panic())
			},
			Entry(
				"an unsupported resource",
				&v1alpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "tcp"}},
			),
			Entry(
				"nil resource",
				nil,
			),
		)

		DescribeTable(
			"CaptureDeleteChange must panic",
			func(resourceType client.Object, nsname types.NamespacedName) {
				process := func() {
					processor.CaptureDeleteChange(resourceType, nsname)
				}
				Expect(process).Should(Panic())
			},
			Entry(
				"an unsupported resource",
				&v1alpha2.TCPRoute{},
				types.NamespacedName{Namespace: "test", Name: "tcp"},
			),
			Entry(
				"nil resource type",
				nil,
				types.NamespacedName{Namespace: "test", Name: "resource"},
			),
		)
	})
})
