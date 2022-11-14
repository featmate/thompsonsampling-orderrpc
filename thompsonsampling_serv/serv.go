package thompsonsampling_serv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	"github.com/go-redis/redis/v8"
	"gonum.org/v1/gonum/stat/distuv"

	log "github.com/Golang-Tools/loggerhelper/v2"
	"github.com/Golang-Tools/optparams"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/admin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	xdscreds "google.golang.org/grpc/credentials/xds"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/xds"

	"net/http"

	_ "embed"

	"github.com/Golang-Tools/etcdhelper/etcdproxy"
	"github.com/Golang-Tools/redishelper/v2/redisproxy"
	"github.com/felixge/httpsnoop"
	"github.com/flowchartsman/swaggerui"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/soheilhy/cmux"
)

// 这边需要指定文件位置
//
//go:embed thompsonsampling.swagger.json
var spec []byte

// Server grpc的服务器结构体
// 服务集成了如下特性:
// 设置收发最大消息长度
// 健康检测
// gzip做消息压缩
// 接口反射
// channelz支持
// TLS支持
// keep alive 支持
type Server struct {
	App_Name    string `json:"app_name,omitempty" jsonschema:"required,description=服务名,default=thompsonsampling"`
	App_Version string `json:"app_version,omitempty" jsonschema:"required,description=服务版本,default=0.0.0"`
	Address     string `json:"address,omitempty" jsonschema:"required,description=服务的主机和端口,default=0.0.0.0:5000"`
	Log_Level   string `json:"log_level,omitempty" jsonschema:"required,description=项目的log等级,enum=TRACE,enum=DEBUG,enum=INFO,enum=WARN,enum=ERROR,default=DEBUG"`

	// 性能设置
	Max_Recv_Msg_Size                           int  `json:"max_recv_msg_size,omitempty" jsonschema:"description=允许接收的最大消息长度"`
	Max_Send_Msg_Size                           int  `json:"max_send_msg_size,omitempty" jsonschema:"description=允许发送的最大消息长度"`
	Initial_Window_Size                         int  `json:"initial_window_size,omitempty" jsonschema:"description=基于Stream的滑动窗口大小"`
	Initial_Conn_Window_Size                    int  `json:"initial_conn_window_size,omitempty" jsonschema:"description=基于Connection的滑动窗口大小"`
	Max_Concurrent_Streams                      int  `json:"max_concurrent_streams,omitempty" jsonschema:"description=一个连接中最大并发Stream数"`
	Max_Connection_Idle                         int  `json:"max_connection_idle,omitempty" jsonschema:"description=客户端连接的最大空闲时长"`
	Max_Connection_Age                          int  `json:"max_connection_age,omitempty" jsonschema:"description=如果连接存活超过n则发送goaway"`
	Max_Connection_Age_Grace                    int  `json:"max_connection_age_grace,omitempty" jsonschema:"description=强制关闭连接之前允许等待的rpc在n秒内完成"`
	Keepalive_Time                              int  `json:"keepalive_time,omitempty" jsonschema:"description=空闲连接每隔n秒ping一次客户端已确保连接存活"`
	Keepalive_Timeout                           int  `json:"keepalive_timeout,omitempty" jsonschema:"description=ping时长超过n则认为连接已死"`
	Keepalive_Enforcement_Min_Time              int  `json:"keepalive_enforement_min_time,omitempty" jsonschema:"description=如果客户端超过每n秒ping一次则终止连接"`
	Keepalive_Enforcement_Permit_Without_Stream bool `json:"keepalive_enforement_permit_without_stream,omitempty" jsonschema:"description=即使没有活动流也允许ping"`

	//TLS设置
	Server_Cert_Path string `json:"server_cert_path,omitempty" jsonschema:"description=使用TLS时服务端的证书位置"`
	Server_Key_Path  string `json:"server_key_path,omitempty" jsonschema:"description=使用TLS时服务端证书的私钥位置"`
	Ca_Cert_Path     string `json:"ca_cert_path,omitempty" jsonschema:"description=使用TLS时根证书位置"`
	Client_Crl_Path  string `json:"client_crl_path,omitempty" jsonschema:"description=客户端证书黑名单位置"`

	//使用XDS
	XDS                  bool `json:"xds,omitempty" jsonschema:"description=是否使用xDSAPIs"`
	XDS_CREDS            bool `json:"xds_creds,omitempty" jsonschema:"description=是否使用xDSAPIs来接收TLS设置"`
	XDS_Maintenance_Port int  `json:"xds_maintenance_port,omitempty" jsonschema:"description=maintenance服务的端口如果不设置则使用当前服务端口+1"`

	// 调试,目前admin接口不稳定,因此只使用channelz
	Use_Admin bool `json:"use_admin,omitempty" jsonschema:"description=是否使用grpc-admin方便调试"`

	UnaryInterceptors  []grpc.UnaryServerInterceptor  `json:"-" jsonschema:"nullable"`
	StreamInterceptors []grpc.StreamServerInterceptor `json:"-" jsonschema:"nullable"`

	//使用grpc-gateway
	Use_Gateway              bool   `json:"use_gateway,omitempty" jsonschema:"description=是否使用grpc-gateway提供restful接口"`
	Gateway_Use_TLS          bool   `json:"gateway_use_tls,omitempty" jsonschema:"description=grpc-gateway是否使用TLS以启动https服务"`
	Gateway_Client_Cert_Path string `json:"gateway_client_cert_path,omitempty" jsonschema:"description=指定gateway访问grpc的客户端证书当XDS_CREDS或设置了TLS设置时生效"`
	Gateway_Client_Key_Path  string `json:"gateway_client_key_path,omitempty" jsonschema:"description=指定gateway访问grpc的客户端证书对应的私钥位置"`

	// redis连接设置
	RedisURL          string `json:"redis_url,omitempty" jsonschema:"required,description=保存点击和未点击数据的redis位置,default=redis://localhost:6379"`
	Redis_RouteMod    string `json:"redis_routemod" jsonschema:"description=redis集群的连接策略,enum=bylatency,enum=randomly,enum=none,default=none"`
	QueryRedisTimeout int    `json:"query_redis_timeout,omitempty" jsonschema:"description=请求redis的超时时长,default=50"`
	DefaultKeyTTL     int    `json:"default_key_ttl,omitempty" jsonschema:"description=保存键的默认过期时间"`

	// 命名空间设置
	Namespace_ALGO                string `json:"namespace_algo,omitempty" jsonschema:"required,description=算法命名空间,default=Tompsonsampling"`
	Namespace_ALGOMeta            string `json:"namespace_algometa,omitempty" jsonschema:"required,description=算法元信息命名空间,default=TompsonsamplingMeta"`
	Namespace_DefaultBusinessName string `json:"namespace_defaultbusinessname,omitempty" jsonschema:"required,description=默认业务名,default=__global__"`
	Namespace_DefaultTargetName   string `json:"namespace_defaulttargetname,omitempty" jsonschema:"required,description=默认目标名,default=__global__"`

	ScopeObserveMode             bool   `json:"scope_observe_mode,omitempty" jsonschema:"description=是否开启范围观测模式"`
	ScopeObserveMode_ETCDURL     string `json:"scope_observe_mode_etcdurl,omitempty" jsonschema:"required,description=范围观测模式使用的etcd路径"`
	ScopeObserveMode_ConnTimeout int    `json:"scope_observe_conntimeout,omitempty" jsonschema:"description=范围观测模式连接的etcd的最大超时单位ms,default=50"`

	thompsonsampling_pb.UnimplementedTHOMPSONSAMPLINGServer `json:"-"`
	opts                                                    []grpc.ServerOption
	healthservice                                           *health.Server
	betapool                                                *sync.Pool
}

// Main 服务的入口函数
func (s *Server) Main() {
	// 初始化log
	log.Set(log.WithLevel(s.Log_Level),
		log.AddExtField("app_name", s.App_Name),
		log.AddExtField("app_version", s.App_Version),
	)
	log.Info("grpc服务获得参数", log.Dict{"ServiceConfig": s})

	//配置redis
	resinitparams := []optparams.Option[redisproxy.Options]{}
	redisaddress := strings.Split(s.RedisURL, ",")
	if len(redisaddress) > 1 {
		copt := redis.ClusterOptions{
			Addrs: redisaddress,
		}
		switch s.Redis_RouteMod {
		case "bylatency":
			{
				copt.RouteByLatency = true
			}
		case "randomly":
			{
				copt.RouteRandomly = true
			}
		}
		resinitparams = append(resinitparams, redisproxy.WithClusterOptions(&copt))
	} else {
		resinitparams = append(resinitparams, redisproxy.WithURL(s.RedisURL))
	}
	if s.QueryRedisTimeout > 0 {
		resinitparams = append(resinitparams, redisproxy.WithQueryTimeoutMS(s.QueryRedisTimeout))
	}
	err := redisproxy.Default.Init(resinitparams...)
	if err != nil {
		log.Error("Init redis get error", log.Dict{"err": err})
		os.Exit(1)
	}
	defer redisproxy.Default.Close()

	//初始化etcd
	if s.ScopeObserveMode {
		initparams := []optparams.Option[etcdproxy.Options]{}
		if s.ScopeObserveMode_ConnTimeout > 0 {
			initparams = append(initparams, etcdproxy.WithQueryTimeout(s.ScopeObserveMode_ConnTimeout))
		}
		err := etcdproxy.Default.Init(s.ScopeObserveMode_ETCDURL, initparams...)
		if err != nil {
			log.Error("Init etcd get error", log.Dict{"err": err})
			os.Exit(1)
		}
		defer etcdproxy.Default.Close()

	}

	// 初始化池
	s.betapool = &sync.Pool{
		New: func() interface{} {
			return new(distuv.Beta)
		},
	}
	s.Run()
}

// PerformanceOpts 配置性能调优设置
func (s *Server) PerformanceOpts() {
	if s.opts == nil {
		s.opts = []grpc.ServerOption{}
	}
	if s.Max_Recv_Msg_Size != 0 {
		s.opts = append(s.opts, grpc.MaxRecvMsgSize(s.Max_Recv_Msg_Size))
	}
	if s.Max_Send_Msg_Size != 0 {
		s.opts = append(s.opts, grpc.MaxSendMsgSize(s.Max_Send_Msg_Size))
	}
	if s.Initial_Window_Size != 0 {
		s.opts = append(s.opts, grpc.InitialWindowSize(int32(s.Initial_Window_Size)))
	}
	if s.Initial_Conn_Window_Size != 0 {
		s.opts = append(s.opts, grpc.InitialConnWindowSize(int32(s.Initial_Conn_Window_Size)))
	}
	if s.Max_Concurrent_Streams != 0 {
		s.opts = append(s.opts, grpc.MaxConcurrentStreams(uint32(s.Max_Concurrent_Streams)))
	}
	if s.Max_Connection_Idle != 0 || s.Max_Connection_Age != 0 || s.Max_Connection_Age_Grace != 0 || s.Keepalive_Time != 0 || s.Keepalive_Timeout != 0 {
		kasp := keepalive.ServerParameters{
			MaxConnectionIdle:     time.Duration(s.Max_Connection_Idle) * time.Second,
			MaxConnectionAge:      time.Duration(s.Max_Connection_Age) * time.Second,
			MaxConnectionAgeGrace: time.Duration(s.Max_Connection_Age_Grace) * time.Second,
			Time:                  time.Duration(s.Keepalive_Time) * time.Second,
			Timeout:               time.Duration(s.Keepalive_Timeout) * time.Second,
		}
		s.opts = append(s.opts, grpc.KeepaliveParams(kasp))
	}
	if s.Keepalive_Enforcement_Min_Time != 0 || s.Keepalive_Enforcement_Permit_Without_Stream {
		kaep := keepalive.EnforcementPolicy{
			MinTime:             time.Duration(s.Keepalive_Enforcement_Min_Time) * time.Second,
			PermitWithoutStream: s.Keepalive_Enforcement_Permit_Without_Stream,
		}
		s.opts = append(s.opts, grpc.KeepaliveEnforcementPolicy(kaep))
	}
}

// TLSOpts 配置TLS设置
func (s *Server) TLSOpts() {
	if s.opts == nil {
		s.opts = []grpc.ServerOption{}
	}
	if s.Ca_Cert_Path != "" {
		cert, err := tls.LoadX509KeyPair(s.Server_Cert_Path, s.Server_Key_Path)
		if err != nil {
			log.Error("read serv pem file error:", log.Dict{"err": err.Error(), "Cert_path": s.Server_Cert_Path, "Key_Path": s.Server_Key_Path})
			os.Exit(2)
		}
		capool := x509.NewCertPool()
		caCrt, err := os.ReadFile(s.Ca_Cert_Path)
		if err != nil {
			log.Error("read ca pem file error:", log.Dict{"err": err.Error(), "path": s.Ca_Cert_Path})
			os.Exit(2)
		}
		capool.AppendCertsFromPEM(caCrt)
		tlsconf := &tls.Config{
			RootCAs:      capool,
			ClientAuth:   tls.RequireAndVerifyClientCert, // 检验客户端证书
			Certificates: []tls.Certificate{cert},
		}
		if s.Client_Crl_Path != "" {
			clipool := x509.NewCertPool()
			cliCrt, err := os.ReadFile(s.Client_Crl_Path)
			if err != nil {
				log.Error("read pem file error:", log.Dict{"err": err.Error(), "path": s.Client_Crl_Path})
				os.Exit(2)
			}
			clipool.AppendCertsFromPEM(cliCrt)
			tlsconf.ClientCAs = clipool
		}
		creds := credentials.NewTLS(tlsconf)
		s.opts = append(s.opts, grpc.Creds(creds))
	} else {
		creds, err := credentials.NewServerTLSFromFile(s.Server_Cert_Path, s.Server_Key_Path)
		if err != nil {
			log.Error("Failed to Listen as a TLS Server", log.Dict{"error": err.Error()})
			os.Exit(2)
		}
		s.opts = append(s.opts, grpc.Creds(creds))
	}
	log.Info("grpc server will start use TLS")
}

func doNothing() {
	log.Debug("Do nothing")
}

// RegistTools 为指定服务注册各种工具
func (s *Server) RegistTools(serv *grpc.Server) func() {

	// 注册健康检查
	s.healthservice = health.NewServer()
	s.healthservice.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(serv, s.healthservice)

	// 注册反射
	reflection.Register(serv)
	// 注册grpc admin
	if s.Use_Admin {
		cleanup, err := admin.Register(serv)
		if err != nil {
			log.Warn("set grpc admin error", log.Dict{"err": err.Error()})
			return doNothing
		}
		return cleanup
	}
	return doNothing
}

// withLogger 记录gateway的工作log
func withLogger(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, writer, request)
		log.Info("Http log", log.Dict{"code": m.Code, "duration": m.Duration, "Path": request.URL.Path})
	})
}

func handleError(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, writer http.ResponseWriter, request *http.Request, err error) {
	var newError runtime.HTTPStatusError
	newError = runtime.HTTPStatusError{
		HTTPStatus: 500,
		Err:        err,
	}
	switch err.Error() {
	case "NotImplemented":
		{
			newError = runtime.HTTPStatusError{
				HTTPStatus: 501,
				Err:        err,
			}
		}
	case "FeatureNotOn":
		{
			newError = runtime.HTTPStatusError{
				HTTPStatus: 503,
				Err:        err,
			}
		}
	default:
		{
			newError = runtime.HTTPStatusError{
				HTTPStatus: 500,
				Err:        err,
			}
		}
	}
	// using default handler to do the rest of heavy lifting of marshaling error and adding headers
	runtime.DefaultHTTPErrorHandler(ctx, mux, marshaler, writer, request, &newError)
}

// MakeGateway 构造grpc-gateway的http服务
func (s *Server) MakeGateway(grpc_need_client_cert bool) *http.Server {
	mux := runtime.NewServeMux(
		runtime.WithErrorHandler(handleError),
	)
	// setting up a dail up for gRPC service by specifying endpoint/target url
	creds := insecure.NewCredentials()
	if grpc_need_client_cert {
		if s.Ca_Cert_Path != "" {
			if s.Gateway_Client_Cert_Path != "" && s.Gateway_Client_Key_Path != "" {
				cert, err := tls.LoadX509KeyPair(s.Gateway_Client_Cert_Path, s.Gateway_Client_Key_Path)
				if err != nil {
					log.Error("read client pem file error:", log.Dict{"err": err.Error(), "Cert_path": s.Gateway_Client_Cert_Path, "Key_Path": s.Gateway_Client_Key_Path})
					os.Exit(1)
				}
				capool := x509.NewCertPool()
				caCrt, err := os.ReadFile(s.Ca_Cert_Path)
				if err != nil {
					log.Error("read ca pem file error:", log.Dict{"err": err.Error(), "path": s.Ca_Cert_Path})
					os.Exit(1)
				}
				capool.AppendCertsFromPEM(caCrt)
				tlsconf := &tls.Config{
					RootCAs:      capool,
					Certificates: []tls.Certificate{cert},
				}
				creds = credentials.NewTLS(tlsconf)

			} else {
				var err error
				creds, err = credentials.NewClientTLSFromFile(s.Ca_Cert_Path, "")
				if err != nil {
					log.Error("failed to load credentials", log.Dict{"err": err.Error()})
					os.Exit(1)
				}
			}
		}
	}
	err := thompsonsampling_pb.RegisterTHOMPSONSAMPLINGHandlerFromEndpoint(context.Background(), mux, s.Address, []grpc.DialOption{grpc.WithTransportCredentials(creds)})
	if err != nil {
		log.Error("Failed to Register GRPCHandlerFromEndpoint", log.Dict{"error": err})
		os.Exit(1)
	}

	m := http.NewServeMux()
	m.Handle("/swagger/", http.StripPrefix("/swagger", swaggerui.Handler(spec)))
	m.Handle("/", withLogger(mux))
	// Creating a normal HTTP server
	server := http.Server{
		Handler: m,
	}
	return &server
}

// RunXdsServer 启动xds服务服务
func (s *Server) RunXdsServer() {
	lis, err := net.Listen("tcp", s.Address)
	if err != nil {
		log.Error("Failed to Listen", log.Dict{"error": err.Error(), "address": s.Address})
		os.Exit(1)
	}
	hostinfo := strings.Split(s.Address, ":")
	if len(hostinfo) != 2 {
		log.Error("address format not ok", log.Dict{"address": s.Address})
		os.Exit(2)
	}
	port, err := strconv.Atoi(hostinfo[1])
	if err != nil {
		log.Error("address port not int", log.Dict{"address": s.Address})
		os.Exit(2)
	}
	mantenancePort := port + 1
	if s.XDS_Maintenance_Port > 0 && s.XDS_Maintenance_Port != port {
		mantenancePort = s.XDS_Maintenance_Port
	}
	mantenanceAddress := fmt.Sprintf("%s:%d", hostinfo[0], mantenancePort)
	mantenanceLis, err := net.Listen("tcp4", mantenanceAddress)
	if err != nil {
		log.Error("Mantenance Service Failed to Listen", log.Dict{"error": err.Error(), "address": mantenanceAddress})
		os.Exit(1)
	}
	maintenanceServer := grpc.NewServer()
	cleanup := s.RegistTools(maintenanceServer)
	defer cleanup()

	creds := insecure.NewCredentials()
	if s.XDS_CREDS {
		var err error
		creds, err = xdscreds.NewServerCredentials(xdscreds.ServerOptions{FallbackCreds: insecure.NewCredentials()})
		if err != nil {
			log.Error("failed to create server-side xDS credentials", log.Dict{"error": err.Error()})
			os.Exit(2)
		}
		log.Info("server will start use XDS_CREDS")
	}
	s.opts = append(s.opts, grpc.Creds(creds))

	gs := xds.NewGRPCServer(s.opts...)
	defer gs.Stop()

	// 注册服务
	thompsonsampling_pb.RegisterTHOMPSONSAMPLINGServer(gs, s)

	if s.Use_Gateway {
		var server *http.Server
		if s.XDS_CREDS {
			server = s.MakeGateway(true)
		} else {
			server = s.MakeGateway(false)
		}
		// 设置分流,http1走restful,http2走grpc
		m := cmux.New(lis)

		// a different listener for HTTP2 since gRPC uses HTTP2
		grpcL := m.Match(cmux.HTTP2())
		// a different listener for HTTP1
		httpL := m.Match(cmux.HTTP1Fast())
		// 构造gateway用户处理http请求

		// 启动服务
		log.Info("Server Start with RESTful and Grpc api and mantenance by XDS", log.Dict{"address": s.Address, "mantenance_port": mantenancePort})
		go func() {
			if s.Gateway_Use_TLS {
				err := server.ServeTLS(httpL, s.Server_Cert_Path, s.Server_Key_Path)
				if err != nil {
					if err.Error() != "mux: server closed" {
						log.Error("Failed to Serve Http", log.Dict{"error": err})
						os.Exit(1)
					}
				}
			} else {
				err := server.Serve(httpL)
				if err != nil {
					if err.Error() != "mux: server closed" {
						log.Error("Failed to Serve Http", log.Dict{"error": err})
						os.Exit(1)
					}
				}
			}
		}()
		go func() {
			err := gs.Serve(grpcL)
			if err != nil {
				log.Error("Failed to Serve Grpc", log.Dict{"error": err})
				os.Exit(1)
			}
		}()
		go func() {
			err := maintenanceServer.Serve(mantenanceLis)
			if err != nil {
				log.Error("Failed to Start Maintenance Server", log.Dict{"error": err})
				os.Exit(1)
			}
		}()
		go func() {
			err := m.Serve()
			if err != nil {
				if !strings.Contains(err.Error(), "use of closed network connection") && err.Error() != "mux: server closed" {
					log.Error("Failed to Start cmux", log.Dict{"error": err})
					os.Exit(1)
				}
			}
		}()
		// 等待中断信号以优雅地关闭服务器（设置 3 秒的超时时间）
		quit := make(chan os.Signal, 3)
		signal.Notify(quit, os.Interrupt)
		<-quit
		log.Info("Shutdown Server ...")
		m.Close()
		gs.GracefulStop()
		maintenanceServer.GracefulStop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = server.Shutdown(ctx)
		if err != nil {
			log.Error("server shutdown error", log.Dict{"err": err.Error()})
			os.Exit(2)
		}
		log.Info("Server Shutdown")
	} else {
		log.Info("Server Start with Grpc api and mantenance by XDS", log.Dict{"address": s.Address, "mantenance_port": mantenancePort})
		go func() {
			err := gs.Serve(lis)
			if err != nil {
				log.Error("Failed to Serve Grpc", log.Dict{"error": err})
				os.Exit(1)
			}
		}()
		go func() {
			err := maintenanceServer.Serve(mantenanceLis)
			if err != nil {
				log.Error("Failed to Start Maintenance Server", log.Dict{"error": err})
				os.Exit(1)
			}
		}()
		// 等待中断信号以优雅地关闭服务器（设置 3 秒的超时时间）
		quit := make(chan os.Signal, 3)
		signal.Notify(quit, os.Interrupt)
		<-quit
		log.Info("Shutdown Server ...")
		gs.GracefulStop()
		maintenanceServer.GracefulStop()
		log.Info("Server Shutdown")
	}
}

// RunCommonServer 启动常规服务服务
func (s *Server) RunCommonServer() {
	lis, err := net.Listen("tcp", s.Address)
	if err != nil {
		log.Error("Failed to Listen", log.Dict{"error": err.Error(), "address": s.Address})
		os.Exit(1)
	}

	if s.Server_Cert_Path != "" && s.Server_Key_Path != "" {
		s.TLSOpts()
	}
	gs := grpc.NewServer(s.opts...)
	defer gs.Stop()
	cleanup := s.RegistTools(gs)
	defer cleanup()

	// 注册服务
	thompsonsampling_pb.RegisterTHOMPSONSAMPLINGServer(gs, s)

	if s.Use_Gateway {
		var server *http.Server
		if s.Server_Cert_Path != "" && s.Server_Key_Path != "" {
			server = s.MakeGateway(true)
		} else {
			server = s.MakeGateway(false)
		}
		// 构造gateway用户处理http请求
		// 设置分流,http1走restful,http2走grpc
		m := cmux.New(lis)
		// a different listener for HTTP2 since gRPC uses HTTP2
		grpcL := m.Match(cmux.HTTP2())
		// a different listener for HTTP1
		httpL := m.Match(cmux.HTTP1Fast())
		// 启动服务
		log.Info("Server Start with RESTful and Grpc api", log.Dict{"address": s.Address})
		go func() {
			err := gs.Serve(grpcL)
			if err != nil {
				log.Error("Failed to Serve Grpc", log.Dict{"error": err})
				os.Exit(1)
			}

		}()
		go func() {
			if s.Gateway_Use_TLS {
				err := server.ServeTLS(httpL, s.Server_Cert_Path, s.Server_Key_Path)
				if err != nil {
					if err.Error() != "mux: server closed" {
						log.Error("Failed to Serve Http", log.Dict{"error": err})
						os.Exit(1)
					}
				}
			} else {
				err := server.Serve(httpL)
				if err != nil {
					if err.Error() != "mux: server closed" {
						log.Error("Failed to Serve Http", log.Dict{"error": err})
						os.Exit(1)
					}
				}
			}
		}()

		go func() {
			err := m.Serve()
			if err != nil {
				if !strings.Contains(err.Error(), "use of closed network connection") && err.Error() != "mux: server closed" {
					log.Error("Failed to Start cmux", log.Dict{"error": err})
					os.Exit(1)
				}
			}
		}()
		// 等待中断信号以优雅地关闭服务器（设置 3 秒的超时时间）
		quit := make(chan os.Signal, 3)
		signal.Notify(quit, os.Interrupt)
		<-quit
		log.Info("Shutdown Server ...")
		m.Close()
		gs.GracefulStop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = server.Shutdown(ctx)
		if err != nil {
			log.Error("server shutdown error", log.Dict{"err": err.Error()})
			os.Exit(2)
		}
		log.Info("Server Shutdown")

	} else {
		// 启动服务
		log.Info("Server Start with Grpc api", log.Dict{"address": s.Address})
		go func() {
			err := gs.Serve(lis)
			if err != nil {
				log.Error("Failed to Serve Grpc", log.Dict{"error": err})
				os.Exit(1)
			}
		}()
		// 等待中断信号以优雅地关闭服务器（设置 3 秒的超时时间）
		quit := make(chan os.Signal, 3)
		signal.Notify(quit, os.Interrupt)
		<-quit
		log.Info("Shutdown Server ...")
		gs.GracefulStop()
		log.Info("Server Shutdown")
	}

}

// RegistInterceptor 注册拦截器
func (s *Server) RegistInterceptor() {
	if s.opts == nil {
		s.opts = []grpc.ServerOption{}
	}
	if len(s.UnaryInterceptors) > 0 {
		for _, i := range s.UnaryInterceptors {
			s.opts = append(s.opts, grpc.UnaryInterceptor(i))
		}
	}
	if len(s.StreamInterceptors) > 0 {
		for _, i := range s.StreamInterceptors {
			s.opts = append(s.opts, grpc.StreamInterceptor(i))
		}
	}
}

// Run 执行grpc服务
func (s *Server) Run() {
	// 注册grpc的配置
	s.PerformanceOpts()
	// 注册拦截器
	s.RegistInterceptor()
	if s.XDS {
		log.Info("setup as xds serv")
		s.RunXdsServer()
	} else {
		log.Info("set up as common serv")
		s.RunCommonServer()
	}
}
