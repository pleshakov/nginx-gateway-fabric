package framework

import "time"

type TimeoutConfig struct {
	// CreateTimeout represents the maximum time for a Kubernetes object to be created.
	CreateTimeout time.Duration

	// UpdateTimeout represents the maximum time for a Kubernetes object to be updated.
	UpdateTimeout time.Duration

	// DeleteTimeout represents the maximum time for a Kubernetes object to be deleted.
	DeleteTimeout time.Duration

	// GetTimeout represents the maximum time to get a Kubernetes object.
	GetTimeout time.Duration

	// ManifestFetchTimeout represents the maximum time for getting content from a https:// URL.
	ManifestFetchTimeout time.Duration

	// RequestTimeout represents the maximum time for making an HTTP Request with the roundtripper.
	RequestTimeout time.Duration
}

// DefaultTimeoutConfig populates a TimeoutConfig with the default values.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		CreateTimeout:        60 * time.Second,
		UpdateTimeout:        60 * time.Second,
		DeleteTimeout:        10 * time.Second,
		GetTimeout:           10 * time.Second,
		ManifestFetchTimeout: 10 * time.Second,
		RequestTimeout:       10 * time.Second,
	}
}
