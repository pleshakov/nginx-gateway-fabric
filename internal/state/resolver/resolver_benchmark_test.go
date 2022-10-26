package resolver

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	discoveryV1 "k8s.io/api/discovery/v1"
)

func generateEndpointSliceList(n int) discoveryV1.EndpointSliceList {
	slicesCount := (n + 99) / 100

	result := discoveryV1.EndpointSliceList{
		Items: make([]discoveryV1.EndpointSlice, 0, slicesCount),
	}

	ready := true

	for i := 0; n > 0; i++ {
		c := 100
		if n < 100 {
			c = n
		}
		n -= 100

		slice := discoveryV1.EndpointSlice{
			Endpoints:   make([]discoveryV1.Endpoint, c, c),
			AddressType: discoveryV1.AddressTypeIPv4,
			Ports: []discoveryV1.EndpointPort{
				{
					Port: nil, // will match any port in the service
				},
			},
		}

		for j := 0; j < c; j++ {
			slice.Endpoints[j] = discoveryV1.Endpoint{
				Addresses: []string{fmt.Sprintf("10.0.%d.%d", i, j)},
				Conditions: discoveryV1.EndpointConditions{
					Ready: &ready,
				},
			}
		}

		result.Items = append(result.Items, slice)
	}

	return result
}

func BenchmarkResolve(b *testing.B) {
	counts := []int{
		1,
		2,
		5,
		10,
		25,
		50,
		100,
		500,
		1000,
	}

	svc := &v1.Service{
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}

	for _, count := range counts {
		list := generateEndpointSliceList(count)

		b.Run(fmt.Sprintf("%d endpoints with original resolve", count), func(b *testing.B) {
			bench(b, svc, list, resolveEndpoints, count)
		})

		b.Run(fmt.Sprintf("%d endpoints with modified resolve", count), func(b *testing.B) {
			bench(b, svc, list, resolveEndpointsSecond, count)
		})
	}
}

type resolveFunc func(*v1.Service, int32, discoveryV1.EndpointSliceList) ([]Endpoint, error)

func bench(b *testing.B, svc *v1.Service, list discoveryV1.EndpointSliceList, resolve resolveFunc, n int) {
	for i := 0; i < b.N; i++ {
		res, err := resolve(svc, 80, list)
		if len(res) != n {
			b.Fatalf("expected %d endpoints, got %d", n, len(res))
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}
