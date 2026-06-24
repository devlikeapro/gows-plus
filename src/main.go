package main

import (
	"flag"
	"github.com/devlikeapro/gows/gows"
	gowsLog "github.com/devlikeapro/gows/log"
	pb "github.com/devlikeapro/gows/proto"
	"github.com/devlikeapro/gows/server"
	"github.com/devlikeapro/gows/wrpc"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/experimental"
	"google.golang.org/grpc/status"
	"net"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

func listenSocket(log waLog.Logger, path string) *net.Listener {
	log.Infof("Server is listening on %s", path)
	// Force remove the socket file
	_ = os.Remove(path)
	// Listen on a specified port
	listener, err := net.Listen("unix", path)
	if err != nil {
		log.Errorf("Failed to listen: %v", err)
	}
	return &listener
}

func buildGrpcServer(log waLog.Logger) *grpc.Server {
	// defines the maximum duration a unary RPC is allowed to run.
	unaryCallTimeout := 30 * time.Minute
	// limit for large media transfers
	maxMessageSizeMb := 128
	maxMessageSize := maxMessageSizeMb * 1024 * 1024
	log.Infof("Maximum gRPC message size set to %d MiB", maxMessageSizeMb)

	// Avoid retaining huge pooled buffers: only reuse up to 1 MiB.
	bufferPool := wrpc.NewCappedBufferPool(1 << 20)

	// Define a custom recovery function to handle panics
	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p interface{}) (err error) {
			stack := debug.Stack()
			log.Errorf("Panic: %v. Stack: %s", p, stack)
			return status.Errorf(codes.Internal, "Internal server error: %v. Stack: %v", p, stack)
		}),
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(recoveryOpts...),
			wrpc.UnaryTimeoutInterceptor(unaryCallTimeout),
		),
		grpc.ChainStreamInterceptor(
			recovery.StreamServerInterceptor(recoveryOpts...),
		),
		grpc.MaxRecvMsgSize(maxMessageSize),
		grpc.MaxSendMsgSize(maxMessageSize),
		experimental.BufferPool(bufferPool),
		grpc.ForceServerCodecV2(wrpc.NewProtoCodec(bufferPool)),
		// Allow more streams and increase window sizes for better performance
		grpc.MaxConcurrentStreams(5000),
		grpc.InitialWindowSize(16*1024*1024),
		grpc.InitialConnWindowSize(32*1024*1024),
	)
	srv := server.NewServer()
	// Add an event handler to the client
	pb.RegisterMessageServiceServer(grpcServer, srv)
	pb.RegisterEventStreamServer(grpcServer, srv)
	return grpcServer
}

var (
	socket             string
	pprofFlag          bool
	pprofPort          int
	pprofHost          string
	pprofBlockRate     int
	pprofMutexFraction int
)

func init() {
	flag.StringVar(&socket, "socket", "/tmp/gows.sock", "Socket path")
	flag.BoolVar(&pprofFlag, "pprof", false, "Enable pprof HTTP server")
	flag.IntVar(&pprofPort, "pprof-port", 6060, "Port for pprof HTTP server")
	flag.StringVar(&pprofHost, "pprof-host", "localhost", "Host for pprof HTTP server")
	flag.IntVar(&pprofBlockRate, "pprof-block-rate", 10_000_000, "Set block profile sampling rate in nanoseconds (pprof only)")
	flag.IntVar(&pprofMutexFraction, "pprof-mutex-fraction", 100, "Set mutex profile sampling rate (1 in N) (pprof only)")
}

func remove(path string) {
	_ = os.Remove(path)
}

func main() {
	flag.Parse()
	log := gowsLog.Stdout("Server", "DEBUG", false)

	// Start pprof HTTP server if enabled
	if pprofFlag {
		if pprofBlockRate > 0 {
			runtime.SetBlockProfileRate(pprofBlockRate)
		}
		if pprofMutexFraction > 0 {
			runtime.SetMutexProfileFraction(pprofMutexFraction)
		}
		StartPprofServer(log, pprofHost, pprofPort)
	}

	clientCfg := getClientConfig()
	log.Infof("Using device name: '%s', browser name: '%s'", clientCfg.DeviceName, clientCfg.BrowserName)
	gows.SetDeviceAndBrowser(clientCfg.DeviceName, clientCfg.BrowserName)
	PatchDeviceProps(gows.GetDeviceProps())

	// Build the server
	grpcServer := buildGrpcServer(log)
	// Open unix socket
	log.Infof("Opening socket %s", socket)
	listener := listenSocket(log, socket)
	defer remove(socket)

	// Start the server
	log.Infof("gRPC server started!")
	if err := grpcServer.Serve(*listener); err != nil {
		log.Errorf("Failed to serve: %v", err)
	}
}
