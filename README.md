# grpcutil
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Facacio%2Fgrpcutil.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Facacio%2Fgrpcutil?ref=badge_shield)

gRPC Utility functions


### Example use

```go
import (
  "github.com/acacio/grpcutil"
  pb "path-to-grpc-proto/proto"
)

type MyServer struct {
  pb.Unimplemented<SOMETHING>Server
}

func NewMyServer(port, grpcwebport string) *MyServer {

  logger := grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, os.Stderr)
  grpclog.SetLoggerV2(logger)

  // Create gRPC server
  srvOpts := grpcutil.ServerOptions()
  s := grpc.NewServer(srvOpts...)

  // OPTIONAL - Registering gRPC reflection for dynamic RPC tools 
  reflection.Register(s)

  // Instantiate service
  mysrv := &MyServer{}
  pb.RegisterMyServer(s, mysrv)

  // OPTIONAL - After your registrations, Prometheus metrics are initialized.
  log.Println("Registering Prometheus...")
  grpc_prometheus.Register(s)

  // Start listening & serving requests
  grpcutil.Serve(s, port) // Fatal failure on error

  return mysrv
}
```

### gRPCWeb

To enable GRPCWeb, before Serve(), you can add the protocol adapter and server in a goroutine:
```go
  // OPTIONAL - Enable gRPCWeb serving. Requires goroutine
  go grpcutil.StartgRPCWeb(s, grpcwebport)
```


## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Facacio%2Fgrpcutil.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Facacio%2Fgrpcutil?ref=badge_large)