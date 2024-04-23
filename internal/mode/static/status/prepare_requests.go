package status

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	ngfAPI "github.com/nginxinc/nginx-gateway-fabric/apis/v1alpha1"
	"github.com/nginxinc/nginx-gateway-fabric/framework/conditions"
	"github.com/nginxinc/nginx-gateway-fabric/framework/helpers"
	frameworkStatus "github.com/nginxinc/nginx-gateway-fabric/framework/status"
	staticConds "github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/conditions"
	"github.com/nginxinc/nginx-gateway-fabric/internal/mode/static/state/graph"
)

// NginxReloadResult describes the result of an NGINX reload.
type NginxReloadResult struct {
	// Error is the error that occurred during the reload.
	Error error
}

// PrepareRouteRequests prepares status UpdateRequests for the given Routes.
func PrepareRouteRequests(
	routes map[types.NamespacedName]*graph.Route,
	transitionTime metav1.Time,
	nginxReloadRes NginxReloadResult,
	gatewayCtlrName string,
) []frameworkStatus.UpdateRequest {
	reqs := make([]frameworkStatus.UpdateRequest, 0, len(routes))

	for nsname, r := range routes {
		parents := make([]v1.RouteParentStatus, 0, len(r.ParentRefs))

		defaultConds := staticConds.NewDefaultRouteConditions()

		for _, ref := range r.ParentRefs {
			failedAttachmentCondCount := 0
			if ref.Attachment != nil && !ref.Attachment.Attached {
				failedAttachmentCondCount = 1
			}
			allConds := make([]conditions.Condition, 0, len(r.Conditions)+len(defaultConds)+failedAttachmentCondCount)

			// We add defaultConds first, so that any additional conditions will override them, which is
			// ensured by DeduplicateConditions.
			allConds = append(allConds, defaultConds...)
			allConds = append(allConds, r.Conditions...)
			if failedAttachmentCondCount == 1 {
				allConds = append(allConds, ref.Attachment.FailedCondition)
			}

			if nginxReloadRes.Error != nil {
				allConds = append(
					allConds,
					staticConds.NewRouteGatewayNotProgrammed(staticConds.RouteMessageFailedNginxReload),
				)
			}

			routeRef := r.Source.Spec.ParentRefs[ref.Idx]

			conds := conditions.DeduplicateConditions(allConds)
			apiConds := conditions.ConvertConditions(conds, r.Source.Generation, transitionTime)

			ps := v1.RouteParentStatus{
				ParentRef: v1.ParentReference{
					Namespace:   helpers.GetPointer(v1.Namespace(ref.Gateway.Namespace)),
					Name:        v1.ObjectName(ref.Gateway.Name),
					SectionName: routeRef.SectionName,
				},
				ControllerName: v1.GatewayController(gatewayCtlrName),
				Conditions:     apiConds,
			}

			parents = append(parents, ps)
		}

		status := v1.HTTPRouteStatus{
			RouteStatus: v1.RouteStatus{
				Parents: parents,
			},
		}

		req := frameworkStatus.UpdateRequest{
			NsName:       nsname,
			ResourceType: &v1.HTTPRoute{},
			Setter:       newHTTPRouteStatusSetter(status, gatewayCtlrName),
		}

		reqs = append(reqs, req)
	}

	return reqs
}

// PrepareGatewayClassRequests prepares status UpdateRequests for the given GatewayClasses.
func PrepareGatewayClassRequests(
	gc *graph.GatewayClass,
	ignoredGwClasses map[types.NamespacedName]*v1.GatewayClass,
	transitionTime metav1.Time,
) []frameworkStatus.UpdateRequest {
	var reqs []frameworkStatus.UpdateRequest

	if gc != nil {
		defaultConds := conditions.NewDefaultGatewayClassConditions()

		conds := make([]conditions.Condition, 0, len(gc.Conditions)+len(defaultConds))

		// We add default conds first, so that any additional conditions will override them, which is
		// ensured by DeduplicateConditions.
		conds = append(conds, defaultConds...)
		conds = append(conds, gc.Conditions...)

		conds = conditions.DeduplicateConditions(conds)

		apiConds := conditions.ConvertConditions(conds, gc.Source.Generation, transitionTime)

		req := frameworkStatus.UpdateRequest{
			NsName:       client.ObjectKeyFromObject(gc.Source),
			ResourceType: &v1.GatewayClass{},
			Setter: newGatewayClassStatusSetter(v1.GatewayClassStatus{
				Conditions: apiConds,
			}),
		}

		reqs = append(reqs, req)
	}

	for nsname, gwClass := range ignoredGwClasses {
		req := frameworkStatus.UpdateRequest{
			NsName:       nsname,
			ResourceType: &v1.GatewayClass{},
			Setter: newGatewayClassStatusSetter(v1.GatewayClassStatus{
				Conditions: conditions.ConvertConditions(
					[]conditions.Condition{conditions.NewGatewayClassConflict()},
					gwClass.Generation,
					transitionTime,
				),
			}),
		}

		reqs = append(reqs, req)
	}

	return reqs
}

// PrepareGatewayRequests prepares status UpdateRequests for the given Gateways.
func PrepareGatewayRequests(
	gateway *graph.Gateway,
	ignoredGateways map[types.NamespacedName]*v1.Gateway,
	transitionTime metav1.Time,
	gwAddresses []v1.GatewayStatusAddress,
	nginxReloadRes NginxReloadResult,
) []frameworkStatus.UpdateRequest {
	reqs := make([]frameworkStatus.UpdateRequest, 0, 1+len(ignoredGateways))

	if gateway != nil {
		reqs = append(reqs, prepareGatewayRequest(gateway, transitionTime, gwAddresses, nginxReloadRes))
	}

	for nsname, gw := range ignoredGateways {
		apiConds := conditions.ConvertConditions(staticConds.NewGatewayConflict(), gw.Generation, transitionTime)
		reqs = append(reqs, frameworkStatus.UpdateRequest{
			NsName:       nsname,
			ResourceType: &v1.Gateway{},
			Setter: newGatewayStatusSetter(v1.GatewayStatus{
				Conditions: apiConds,
			}),
		})
	}

	return reqs
}

func prepareGatewayRequest(
	gateway *graph.Gateway,
	transitionTime metav1.Time,
	gwAddresses []v1.GatewayStatusAddress,
	nginxReloadRes NginxReloadResult,
) frameworkStatus.UpdateRequest {
	if !gateway.Valid {
		conds := conditions.ConvertConditions(
			conditions.DeduplicateConditions(gateway.Conditions),
			gateway.Source.Generation,
			transitionTime,
		)

		return frameworkStatus.UpdateRequest{
			NsName:       client.ObjectKeyFromObject(gateway.Source),
			ResourceType: &v1.Gateway{},
			Setter: newGatewayStatusSetter(v1.GatewayStatus{
				Conditions: conds,
			}),
		}
	}

	listenerStatuses := make([]v1.ListenerStatus, 0, len(gateway.Listeners))

	validListenerCount := 0
	for _, l := range gateway.Listeners {
		var conds []conditions.Condition

		if l.Valid {
			conds = staticConds.NewDefaultListenerConditions()
			validListenerCount++
		} else {
			conds = l.Conditions
		}

		if nginxReloadRes.Error != nil {
			conds = append(
				conds,
				staticConds.NewListenerNotProgrammedInvalid(staticConds.ListenerMessageFailedNginxReload),
			)
		}

		apiConds := conditions.ConvertConditions(
			conditions.DeduplicateConditions(conds),
			gateway.Source.Generation,
			transitionTime,
		)

		listenerStatuses = append(listenerStatuses, v1.ListenerStatus{
			Name:           v1.SectionName(l.Name),
			SupportedKinds: l.SupportedKinds,
			AttachedRoutes: int32(len(l.Routes)),
			Conditions:     apiConds,
		})
	}

	gwConds := staticConds.NewDefaultGatewayConditions()
	if validListenerCount == 0 {
		gwConds = append(gwConds, staticConds.NewGatewayNotAcceptedListenersNotValid()...)
	} else if validListenerCount < len(gateway.Listeners) {
		gwConds = append(gwConds, staticConds.NewGatewayAcceptedListenersNotValid())
	}

	if nginxReloadRes.Error != nil {
		gwConds = append(
			gwConds,
			staticConds.NewGatewayNotProgrammedInvalid(staticConds.GatewayMessageFailedNginxReload),
		)
	}

	apiGwConds := conditions.ConvertConditions(
		conditions.DeduplicateConditions(gwConds),
		gateway.Source.Generation,
		transitionTime,
	)

	return frameworkStatus.UpdateRequest{
		NsName:       client.ObjectKeyFromObject(gateway.Source),
		ResourceType: &v1.Gateway{},
		Setter: newGatewayStatusSetter(v1.GatewayStatus{
			Listeners:  listenerStatuses,
			Conditions: apiGwConds,
			Addresses:  gwAddresses,
		}),
	}
}

// PrepareBackendTLSPolicyRequests prepares status UpdateRequests for the given BackendTLSPolicies.
func PrepareBackendTLSPolicyRequests(
	policies map[types.NamespacedName]*graph.BackendTLSPolicy,
	transitionTime metav1.Time,
	gatewayCtlrName string,
) []frameworkStatus.UpdateRequest {
	reqs := make([]frameworkStatus.UpdateRequest, 0, len(policies))

	for nsname, pol := range policies {
		if !pol.IsReferenced || pol.Ignored {
			continue
		}

		conds := conditions.DeduplicateConditions(pol.Conditions)
		apiConds := conditions.ConvertConditions(conds, pol.Source.Generation, transitionTime)

		status := v1alpha2.PolicyStatus{
			Ancestors: []v1alpha2.PolicyAncestorStatus{
				{
					AncestorRef: v1.ParentReference{
						Namespace: (*v1.Namespace)(&pol.Gateway.Namespace),
						Name:      v1alpha2.ObjectName(pol.Gateway.Name),
					},
					ControllerName: v1alpha2.GatewayController(gatewayCtlrName),
					Conditions:     apiConds,
				},
			},
		}

		reqs = append(reqs, frameworkStatus.UpdateRequest{
			NsName:       nsname,
			ResourceType: &v1alpha2.BackendTLSPolicy{},
			Setter:       newBackendTLSPolicyStatusSetter(status, gatewayCtlrName),
		})
	}
	return reqs
}

// ControlPlaneUpdateResult describes the result of a control plane update.
type ControlPlaneUpdateResult struct {
	// Error is the error that occurred during the update.
	Error error
}

// PrepareNginxGatewayStatus prepares a status UpdateRequest for the given NginxGateway.
// If the NginxGateway is nil, it returns nil.
func PrepareNginxGatewayStatus(
	nginxGateway *ngfAPI.NginxGateway,
	transitionTime metav1.Time,
	cpUpdateRes ControlPlaneUpdateResult,
) *frameworkStatus.UpdateRequest {
	if nginxGateway == nil {
		return nil
	}

	var conds []conditions.Condition
	if cpUpdateRes.Error != nil {
		msg := "Failed to update control plane configuration"
		conds = []conditions.Condition{staticConds.NewNginxGatewayInvalid(fmt.Sprintf("%s: %v", msg, cpUpdateRes.Error))}
	} else {
		conds = []conditions.Condition{staticConds.NewNginxGatewayValid()}
	}

	return &frameworkStatus.UpdateRequest{
		NsName:       client.ObjectKeyFromObject(nginxGateway),
		ResourceType: &ngfAPI.NginxGateway{},
		Setter: newNginxGatewayStatusSetter(ngfAPI.NginxGatewayStatus{
			Conditions: conditions.ConvertConditions(conds, nginxGateway.Generation, transitionTime),
		}),
	}
}
