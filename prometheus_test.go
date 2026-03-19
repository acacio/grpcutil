package grpcutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func TestEnablePrometheus(t *testing.T) {
	s := grpc.NewServer()
	h := EnablePrometheus(s, "9090")
	if h == nil {
		t.Error("EnablePrometheus returned nil handler")
	}

	req, err := http.NewRequest("GET", "/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestGetMethod(t *testing.T) {
	name := "grpc_method"
	val := "/foo.Bar/Get"
	labels := []*dto.LabelPair{{Name: &name, Value: &val}}
	if got := getMethod(labels); got != val {
		t.Errorf("expected %s, got %s", val, got)
	}
	if got := getMethod(nil); got != "UNKNOWN" {
		t.Errorf("expected UNKNOWN, got %s", got)
	}
}

func TestPrometheusMetricsWithData(t *testing.T) {
	s := grpc.NewServer()
	EnablePrometheus(s, "9090") // registers s

	lis := bufconn.Listen(1024 * 1024)
	grpc_health_v1.RegisterHealthServer(s, health.NewServer())
	go s.Serve(lis)
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(bufDialer(lis)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	_, err = hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	// Give prometheus a moment to record the metrics
	time.Sleep(50 * time.Millisecond)

	metrics := GetgRPCMetrics()
	if len(metrics) == 0 {
		t.Log("GetgRPCMetrics returned empty map")
	}

	histograms := GetgRPCHistograms()
	if len(histograms) == 0 {
		t.Log("GetgRPCHistograms returned empty map")
	}
}
