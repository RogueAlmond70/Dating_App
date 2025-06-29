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
			Name:      "list_liked_you_requests_total",
			Help:      "Counter of requests to explore-service for a list of users that liked recipient",
		},
		[]string{"request"},
	)
	ListLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "list_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service for list of users that liked recipient",
		},
		[]string{"request_error"},
	)
	ListLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "list_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for list liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
	ListNewLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "list_new_liked_you_requests_total",
			Help:      "Counter of requests to explore-service to list all users who liked the recipient excluding those who have been liked in return",
		},
		[]string{"request"},
	)
	ListNewLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "list_new_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service to list all users who liked the recipient excluding those who have been liked in return",
		},
		[]string{"request_error"},
	)
	ListNewLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "list_new_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for list of new liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)

	CountLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "count_liked_you_requests_total",
			Help:      "Counter of requests to explore-service for a list of users that liked recipient", // Record the decision of the actor to like or pass the recipient
		},
		[]string{"request"},
	)
	CountLikedYouRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "count_liked_you_request_errors_total",
			Help:      "Counter of errored requests to explore-service for a list of users that liked recipient", // Record the decision of the actor to like or pass the recipient
		},
		[]string{"request_error"},
	)
	CountLikedYouRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "count_liked_you_requests_duration_seconds",
			Help:      "Latency histogram for count liked you calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
	PutDecisionRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "put_decision_requests_total",
			Help:      "Counter of requests to explore-service to check mutual likes",
		},
		[]string{"request"},
	)
	PutDecisionMutualLikesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "put_decision_mutual_matches_total",
			Help:      "Counter of mutual matches",
		},
		[]string{"counter"},
	)
	PutDecisionNoMutualLikesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "put_decision_no_mutual_matches_total",
			Help:      "Counter of non mutual matches",
		},
		[]string{"counter"},
	)
	PutDecisionRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "put_decision_request_errors_total",
			Help:      "Counter of errored requests to explore-service to check mutual likes",
		},
		[]string{"request_error"},
	)
	PutDecisionRequestDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "put_decision_requests_duration_seconds",
			Help:      "Latency histogram for put decision calls",
			Buckets:   prometheus.DefBuckets,
		},
	)
)
