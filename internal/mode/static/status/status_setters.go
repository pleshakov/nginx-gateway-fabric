package status

import (
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	ngfAPI "github.com/nginxinc/nginx-gateway-fabric/apis/v1alpha1"
	"github.com/nginxinc/nginx-gateway-fabric/framework/helpers"
	frameworkStatus "github.com/nginxinc/nginx-gateway-fabric/framework/status"
)

func newNginxGatewayStatusSetter(status ngfAPI.NginxGatewayStatus) frameworkStatus.Setter {
	return func(obj client.Object) (wasSet bool) {
		ng := helpers.MustCastObject[*ngfAPI.NginxGateway](obj)

		if frameworkStatus.ConditionsEqual(ng.Status.Conditions, status.Conditions) {
			return false
		}

		ng.Status = status
		return true
	}
}

func newGatewayStatusSetter(status gatewayv1.GatewayStatus) frameworkStatus.Setter {
	return func(obj client.Object) (wasSet bool) {
		gw := helpers.MustCastObject[*gatewayv1.Gateway](obj)

		if gwStatusEqual(gw.Status, status) {
			return false
		}

		gw.Status = status
		return true
	}
}

func gwStatusEqual(prev, cur gatewayv1.GatewayStatus) bool {
	addressesEqual := slices.EqualFunc(prev.Addresses, cur.Addresses, func(a1, a2 gatewayv1.GatewayStatusAddress) bool {
		if !equalPointers[gatewayv1.AddressType](a1.Type, a2.Type) {
			return false
		}

		return a1.Value == a2.Value
	})

	if !addressesEqual {
		return false
	}

	if !frameworkStatus.ConditionsEqual(prev.Conditions, cur.Conditions) {
		return false
	}

	return slices.EqualFunc(prev.Listeners, cur.Listeners, func(s1, s2 gatewayv1.ListenerStatus) bool {
		if s1.Name != s2.Name {
			return false
		}

		if s1.AttachedRoutes != s2.AttachedRoutes {
			return false
		}

		if !frameworkStatus.ConditionsEqual(s1.Conditions, s2.Conditions) {
			return false
		}

		return slices.EqualFunc(s1.SupportedKinds, s2.SupportedKinds, func(k1, k2 gatewayv1.RouteGroupKind) bool {
			if k1.Kind != k2.Kind {
				return false
			}

			return equalPointers(k1.Group, k2.Group)
		})
	})
}

func newHTTPRouteStatusSetter(status gatewayv1.HTTPRouteStatus, gatewayCtlrName string) frameworkStatus.Setter {
	return func(object client.Object) (wasSet bool) {
		hr := object.(*gatewayv1.HTTPRoute)

		// keep all the parent statuses that belong to other controllers
		for _, os := range hr.Status.Parents {
			if string(os.ControllerName) != gatewayCtlrName {
				status.Parents = append(status.Parents, os)
			}
		}

		if hrStatusEqual(gatewayCtlrName, hr.Status, status) {
			return false
		}

		hr.Status = status

		return true
	}
}

func hrStatusEqual(gatewayCtlrName string, prev, cur gatewayv1.HTTPRouteStatus) bool {
	// Since other controllers may update HTTPRoute status we can't assume anything about the order of the statuses,
	// and we have to ignore statuses written by other controllers when checking for equality.
	// Therefore, we can't use slices.EqualFunc here because it cares about the order.

	// First, we check if the prev status has any RouteParentStatuses that are no longer present in the cur status.
	for _, prevParent := range prev.Parents {
		if prevParent.ControllerName != gatewayv1.GatewayController(gatewayCtlrName) {
			continue
		}

		exists := slices.ContainsFunc(cur.Parents, func(curParent gatewayv1.RouteParentStatus) bool {
			return routeParentStatusEqual(prevParent, curParent)
		})

		if !exists {
			return false
		}
	}

	// Then, we check if the cur status has any RouteParentStatuses that are no longer present in the prev status.
	for _, curParent := range cur.Parents {
		exists := slices.ContainsFunc(prev.Parents, func(prevParent gatewayv1.RouteParentStatus) bool {
			return routeParentStatusEqual(curParent, prevParent)
		})

		if !exists {
			return false
		}
	}

	return true
}

func routeParentStatusEqual(p1, p2 gatewayv1.RouteParentStatus) bool {
	if p1.ControllerName != p2.ControllerName {
		return false
	}

	if p1.ParentRef.Name != p2.ParentRef.Name {
		return false
	}

	if !equalPointers(p1.ParentRef.Namespace, p2.ParentRef.Namespace) {
		return false
	}

	if !equalPointers(p1.ParentRef.SectionName, p2.ParentRef.SectionName) {
		return false
	}

	// we ignore the rest of the ParentRef fields because we do not set them

	return frameworkStatus.ConditionsEqual(p1.Conditions, p2.Conditions)
}

func newGatewayClassStatusSetter(status gatewayv1.GatewayClassStatus) frameworkStatus.Setter {
	return func(obj client.Object) (wasSet bool) {
		gc := helpers.MustCastObject[*gatewayv1.GatewayClass](obj)

		if frameworkStatus.ConditionsEqual(gc.Status.Conditions, status.Conditions) {
			return false
		}

		gc.Status = status
		return true
	}
}

func newBackendTLSPolicyStatusSetter(
	status gatewayv1alpha2.PolicyStatus,
	gatewayCtlrName string,
) frameworkStatus.Setter {
	return func(object client.Object) (wasSet bool) {
		btp := helpers.MustCastObject[*gatewayv1alpha2.BackendTLSPolicy](object)

		// maxAncestors is the max number of ancestor statuses which is the sum of all new ancestor statuses and all old
		// ancestor statuses.
		maxAncestors := 1 + len(btp.Status.Ancestors)
		ancestors := make([]gatewayv1alpha2.PolicyAncestorStatus, 0, maxAncestors)

		// keep all the ancestor statuses that belong to other controllers
		for _, os := range btp.Status.Ancestors {
			if string(os.ControllerName) != gatewayCtlrName {
				ancestors = append(ancestors, os)
			}
		}

		ancestors = append(ancestors, status.Ancestors...)
		status.Ancestors = ancestors

		if btpStatusEqual(gatewayCtlrName, btp.Status, status) {
			return false
		}

		btp.Status = status
		return true
	}
}

func btpStatusEqual(gatewayCtlrName string, prev, cur gatewayv1alpha2.PolicyStatus) bool {
	// Since other controllers may update BackendTLSPolicy status we can't assume anything about the order of the
	// statuses, and we have to ignore statuses written by other controllers when checking for equality.
	// Therefore, we can't use slices.EqualFunc here because it cares about the order.

	// First, we check if the prev status has any PolicyAncestorStatuses that are no longer present in the cur status.
	for _, prevAncestor := range prev.Ancestors {
		if prevAncestor.ControllerName != gatewayv1.GatewayController(gatewayCtlrName) {
			continue
		}

		exists := slices.ContainsFunc(cur.Ancestors, func(curAncestor gatewayv1alpha2.PolicyAncestorStatus) bool {
			return btpAncestorStatusEqual(prevAncestor, curAncestor)
		})

		if !exists {
			return false
		}
	}

	// Then, we check if the cur status has any PolicyAncestorStatuses that are no longer present in the prev status.
	for _, curParent := range cur.Ancestors {
		exists := slices.ContainsFunc(prev.Ancestors, func(prevAncestor gatewayv1alpha2.PolicyAncestorStatus) bool {
			return btpAncestorStatusEqual(curParent, prevAncestor)
		})

		if !exists {
			return false
		}
	}

	return true
}

func btpAncestorStatusEqual(p1, p2 gatewayv1alpha2.PolicyAncestorStatus) bool {
	if p1.ControllerName != p2.ControllerName {
		return false
	}

	if p1.AncestorRef.Name != p2.AncestorRef.Name {
		return false
	}

	if !equalPointers(p1.AncestorRef.Namespace, p2.AncestorRef.Namespace) {
		return false
	}

	// we ignore the rest of the AncestorRef fields because we do not set them

	return frameworkStatus.ConditionsEqual(p1.Conditions, p2.Conditions)
}

// equalPointers returns whether two pointers are equal.
// Pointers are considered equal if one of the following is true:
// - They are both nil.
// - One is nil and the other is empty (e.g. nil string and "").
// - They are both non-nil, and their values are the same.
func equalPointers[T comparable](p1, p2 *T) bool {
	if p1 == nil && p2 == nil {
		return true
	}

	var p1Val, p2Val T

	if p1 != nil {
		p1Val = *p1
	}

	if p2 != nil {
		p2Val = *p2
	}

	return p1Val == p2Val
}
