package prometheus

import (
	"context"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/prometheus/client_golang/prometheus"
	prometheusclient "github.com/prometheus/client_golang/prometheus"
)

const (
	existStatusFailure = "failure"
	exitStatusSuccess  = "success"
)

func ResolverMiddleware(prom prometheus.Registerer) graphql.FieldMiddleware {

	resolverInflightGauge := prometheusclient.NewGaugeVec(
		prometheusclient.GaugeOpts{
			Name: "graphql_server_inflight_resolvers",
			Help: "Active resolvers.",
		},
		[]string{"object", "field"},
	)

	resolversCounter := prometheusclient.NewCounterVec(
		prometheusclient.CounterOpts{
			Name: "graphql_server_resovler_executions_total",
			Help: "Total number of resolvers executed since start-up.",
		},
		[]string{"object", "field"},
	)

	timeToResolveField := prometheusclient.NewHistogramVec(prometheusclient.HistogramOpts{
		Name:    "graphql_server_resolver_duration_ms",
		Help:    "The time taken to resolve a field by graphql server.",
		Buckets: prometheusclient.ExponentialBuckets(0.001, 2, 4),
	}, []string{"exitStatus"})

	prom.MustRegister(
		resolverInflightGauge,
		resolversCounter,
		timeToResolveField,
	)

	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		rctx := graphql.GetResolverContext(ctx)

		resolversCounter.WithLabelValues(rctx.Object, rctx.Field.Name).Inc()
		resolverInflightGauge.WithLabelValues(rctx.Object, rctx.Field.Name).Inc()
		defer resolverInflightGauge.WithLabelValues(rctx.Object, rctx.Field.Name).Dec()

		observerStart := time.Now()

		res, err := next(ctx)

		var exitStatus string
		if err != nil {
			exitStatus = existStatusFailure
		} else {
			exitStatus = exitStatusSuccess
		}

		timeToResolveField.With(prometheusclient.Labels{"exitStatus": exitStatus}).
			Observe(float64(time.Since(observerStart)) / float64(time.Second))

		return res, err
	}
}

func RequestMiddleware(prom prometheus.Registerer) graphql.RequestMiddleware {
	requestsInflightGauge := prometheusclient.NewGauge(
		prometheusclient.GaugeOpts{
			Name: "graphql_server_inflight_requests",
			Help: "Active requests.",
		},
	)

	requestsCounter := prometheusclient.NewCounter(
		prometheusclient.CounterOpts{
			Name: "graphql_server_requests_total",
			Help: "Total number of requests arrived since start-up.",
		},
	)

	timeToHandleRequest := prometheusclient.NewHistogramVec(prometheusclient.HistogramOpts{
		Name:    "graphql_server_request_duration_seconds",
		Help:    "The time taken to handle a request by graphql server.",
		Buckets: prometheusclient.ExponentialBuckets(0.001, 2, 4),
	}, []string{"exitStatus"})
	prometheusclient.MustRegister(
		requestsInflightGauge,
		requestsCounter,
		timeToHandleRequest,
	)
	return func(ctx context.Context, next func(ctx context.Context) []byte) []byte {
		requestsInflightGauge.Inc()
		defer requestsInflightGauge.Dec()

		requestsCounter.Inc()

		observerStart := time.Now()

		res := next(ctx)

		rctx := graphql.GetResolverContext(ctx)
		reqCtx := graphql.GetRequestContext(ctx)
		errList := reqCtx.GetErrors(rctx)

		var exitStatus string
		if len(errList) > 0 {
			exitStatus = existStatusFailure
		} else {
			exitStatus = exitStatusSuccess
		}

		timeToHandleRequest.With(prometheusclient.Labels{"exitStatus": exitStatus}).
			Observe(float64(time.Since(observerStart)) / float64(time.Second))

		return res
	}
}
