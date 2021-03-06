package base

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var Registry *prometheus.Registry
var Once sync.Once

var IncomingMsgsCounter *prometheus.CounterVec
var ClientConnectionCounter *prometheus.CounterVec
var ParsingErrorCounter *prometheus.CounterVec

func InitRegistry() {
	IncomingMsgsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skw_incoming_messages_total",
			Help: "total number of messages that were received",
		},
		[]string{"provider", "client", "port", "path"},
	)

	ClientConnectionCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skw_client_connections_total",
			Help: "total number of client connections",
		},
		[]string{"provider", "client", "port", "path"},
	)

	ParsingErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skw_parsing_errors_total",
			Help: "total number of times there was a parsing error",
		},
		[]string{"provider", "client", "parsername"},
	)

	Registry = prometheus.NewRegistry()
	Registry.MustRegister(
		ClientConnectionCounter,
		IncomingMsgsCounter,
		ParsingErrorCounter,
	)
}
