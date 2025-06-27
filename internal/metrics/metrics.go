package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "explore-service"

// Metrics for the explore-service used to observe the behaviour of the service.
// These metrics should be updated by the caller of the service rather than by the service itself.

var (
	ListLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_liked_you_requests_total",
			Help:      "Counter of requests to explore-service for a list of users that liked recipient",
		},
		[]string{},
	)
	ListLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service for list of users that liked recipient",
		},
		[]string{"error_type"},
	)
	ListLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for list liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
	ListNewLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_new_liked_you_requests_total",
			Help:      "Counter of requests to explore-service to list all users who liked the recipient excluding those who have been liked in return",
		},
		[]string{},
	)
	ListNewLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_new_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service to list all users who liked the recipient excluding those who have been liked in return",
		},
		[]string{"error_type"},
	)
	ListNewLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_new_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for list of new liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)

	CountLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "count_liked_you_requests_total",
			Help:      "Counter of requests to explore-service for a list of users that liked recipient", // Record the decision of the actor to like or pass the recipient
		},
		[]string{},
	)
	CountLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "count_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service for a list of users that liked recipient", // Record the decision of the actor to like or pass the recipient
		},
		[]string{"error_type"},
	)
	CountLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "count_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for count liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
	PutDecisionRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "put_decision_requests_total",
			Help:      "Counter of requests to explore-service to check mutual likes",
		},
		[]string{},
	)
	PutDecisionRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "put_decision_request_errors_total",
			Help:      "Counter of errored requests to explore-service to check mutual likes",
		},
		[]string{"error_type"},
	)
	PutDecisionRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "put_decision_requests_duration_seconds",
			Help:      "Latency histogram for put decision calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
)
