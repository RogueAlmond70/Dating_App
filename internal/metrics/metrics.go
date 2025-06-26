package metrics

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"sync"
	"time"
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
		[]string{namespace, "likes"},
	)
	ListNewLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_new_liked_you_requests_total",
			Help:      "Counter of requests to explore-service to list all users who liked the recipient excluding those who have been liked in return",
		},
		[]string{namespace, "likes"},
	)

	CountLikedYouRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "likes",
			Name:      "list_new_liked_you_requests_total",
			Help:      "Counter of requests to explore-service for a list of users that liked recipient", // Record the decision of the actor to like or pass the recipient
		},
		[]string{namespace, "likes"},
	)
)
