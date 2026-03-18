/*
Copyright 2021 Acacio Cruz acacio@acacio.coom

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package grpcutil

import (
	"net/http"
	_ "net/http/pprof" // registers "/debug/pprof" handlers; trace via:
	// curl http://localhost:9999/debug/pprof/trace?seconds=5 -o trace.out
	// go tool trace trace.out

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"
)

// EnablePrometheus registers the gRPC server with Prometheus, enables handling-time
// histograms, and returns an HTTP handler for the /metrics endpoint.
func EnablePrometheus(s *grpc.Server, PORT string) http.Handler {
	grpc_prometheus.Register(s)
	grpc_prometheus.EnableHandlingTimeHistogram()
	return promhttp.Handler()
}

// GetgRPCMetrics returns average latency in milliseconds per gRPC method name.
func GetgRPCMetrics() map[string]float64 {
	metrics := grpc_prometheus.DefaultServerMetrics

	lats := make(map[string]float64)

	c := make(chan prometheus.Metric)
	go func() {
		metrics.Collect(c)
		close(c)
	}()

	for metric := range c {
		data := &dto.Metric{}
		metric.Write(data)
		if h := data.GetHistogram(); h != nil {
			count := h.GetSampleCount()
			sum := h.GetSampleSum()
			if count > 0 {
				latency := (1000.0 * sum) / float64(count) // seconds → milliseconds
				method := getMethod(data.GetLabel())
				lats[method] = latency
			}
		}
	}
	return lats
}

// GetgRPCHistograms returns per-method histogram bucket distributions.
// Each bucket maps its upper bound to the differential count (not cumulative).
func GetgRPCHistograms() map[string]map[float64]uint64 {
	metrics := grpc_prometheus.DefaultServerMetrics

	c := make(chan prometheus.Metric)
	go func() {
		metrics.Collect(c)
		close(c)
	}()

	histPerMethod := make(map[string]map[float64]uint64)
	for metric := range c {
		data := &dto.Metric{}
		metric.Write(data)
		if h := data.GetHistogram(); h != nil {
			method := getMethod(data.GetLabel())
			hist := make(map[float64]uint64)
			var prev uint64 = 0
			for _, b := range h.GetBucket() {
				max := b.GetUpperBound()
				cnt := b.GetCumulativeCount()
				hist[max] = cnt - prev // differential count per bucket
				prev = cnt
			}
			histPerMethod[method] = hist
		}
	}
	return histPerMethod
}

// getMethod extracts the grpc_method label value from a set of Prometheus label pairs.
func getMethod(labels []*dto.LabelPair) string {
	for _, l := range labels {
		if l.GetName() == "grpc_method" {
			return l.GetValue()
		}
	}
	return "UNKNOWN"
}
