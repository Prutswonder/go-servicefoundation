package servicefoundation

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Prutswonder/go-servicefoundation/env"
)

const (
	envCORSOrigins       string = "CORS_ORIGINS"
	envHTTPpPort         string = "HTTPPORT"
	envLogMinFilter      string = "LOG_MINFILTER"
	envAppName           string = "APP_NAME"
	envServerName        string = "SERVER_NAME"
	envDeployEnvironment string = "DEPLOY_ENVIRONMENT"

	defaultHTTPPort     int    = 8080
	defaultLogMinFilter string = "Warning"

	publicSubsystem = "public"
)

type (
	// ShutdownFunc is a function signature for the shutdown function.
	ShutdownFunc func(log Logger)

	// ServiceGlobals contains basic service properties, like name, deployment environment and version number.
	ServiceGlobals struct {
		AppName           string
		ServerName        string
		DeployEnvironment string
		VersionNumber     string
	}

	// ServiceOptions contains value and references used by the Service implementation. The contents of ServiceOptions
	// can be used to customize or extend ServiceFoundation.
	ServiceOptions struct {
		Globals            ServiceGlobals
		Port               int
		ReadinessPort      int
		InternalPort       int
		Logger             Logger
		Metrics            Metrics
		RouterFactory      RouterFactory
		MiddlewareWrapper  MiddlewareWrapper
		Handlers           *Handlers
		WrapHandler        WrapHandler
		VersionBuilder     VersionBuilder
		ServiceStateReader ServiceStateReader
		ShutdownFunc       ShutdownFunc
		ExitFunc           ExitFunc
		ServerTimeout      time.Duration
	}

	// ServiceStateReader contains state methods used by the service's handler implementations.
	ServiceStateReader interface {
		IsLive() bool
		IsReady() bool
		IsHealthy() bool
	}

	// Service is the main interface for ServiceFoundation and is used to define routing and running the service.
	Service interface {
		Run(ctx context.Context)
		AddRoute(name string, routes []string, methods []string, middlewares []Middleware, handler Handle)
	}

	serviceStateReaderImpl struct {
	}

	serviceImpl struct {
		globals         ServiceGlobals
		serverTimeout   time.Duration
		port            int
		readinessPort   int
		internalPort    int
		log             Logger
		metrics         Metrics
		publicRouter    *Router
		readinessRouter *Router
		internalRouter  *Router
		handlers        *Handlers
		wrapHandler     WrapHandler
		versionBuilder  VersionBuilder
		stateReader     ServiceStateReader
		shutdownFunc    ShutdownFunc
		exitFunc        ExitFunc
		quitting        bool
		sendChan        chan bool
		receiveChan     chan bool
	}

	serverInstance struct {
		shutdownChan chan bool
	}
)

// DefaultMiddlewares contains the default middleware wrappers for the predefined service endpoints.
var DefaultMiddlewares = []Middleware{PanicTo500, RequestLogging, NoCaching}

// NewService creates and returns a Service that uses environment variables for default configuration.
func NewService(name string, allowedMethods []string, shutdownFunc ShutdownFunc) Service {
	opt := NewServiceOptions(name, allowedMethods, shutdownFunc)

	return NewCustomService(opt)
}

// NewServiceOptions creates and returns ServiceOptions that use environment variables for default configuration.
func NewServiceOptions(name string, allowedMethods []string, shutdownFunc ShutdownFunc) ServiceOptions {
	appName := env.OrDefault(envAppName, name)
	serverName := env.OrDefault(envServerName, name)
	deployEnvironment := env.OrDefault(envDeployEnvironment, "UNKNOWN")
	corsOptions := CORSOptions{
		AllowedOrigins: env.ListOrDefault(envCORSOrigins, []string{"*"}),
		AllowedMethods: allowedMethods,
	}
	logger := NewLogger(env.OrDefault(envLogMinFilter, defaultLogMinFilter))
	metrics := NewMetrics(name, logger)
	versionBuilder := NewVersionBuilder()
	version := NewBuildVersion()
	globals := ServiceGlobals{
		AppName:           appName,
		ServerName:        serverName,
		DeployEnvironment: deployEnvironment,
		VersionNumber:     version.VersionNumber,
	}
	middlewareWrapper := NewMiddlewareWrapper(logger, metrics, &corsOptions, globals)
	stateReader := NewServiceStateReader()
	exitFunc := NewExitFunc(logger, shutdownFunc)
	port := env.AsInt(envHTTPpPort, defaultHTTPPort)

	opt := ServiceOptions{
		Globals:            globals,
		ServerTimeout:      time.Second * 20,
		Port:               port,
		ReadinessPort:      port + 1,
		InternalPort:       port + 2,
		MiddlewareWrapper:  middlewareWrapper,
		RouterFactory:      NewRouterFactory(),
		Logger:             logger,
		Metrics:            metrics,
		VersionBuilder:     versionBuilder,
		ServiceStateReader: stateReader,
		ExitFunc:           exitFunc,
	}
	opt.SetHandlers()
	return opt
}

// NewCustomService allows you to customize ServiceFoundation using your own implementations of factories.
func NewCustomService(options ServiceOptions) Service {
	return &serviceImpl{
		globals:         options.Globals,
		serverTimeout:   options.ServerTimeout,
		port:            options.Port,
		readinessPort:   options.ReadinessPort,
		internalPort:    options.InternalPort,
		log:             options.Logger,
		metrics:         options.Metrics,
		publicRouter:    options.RouterFactory.NewRouter(),
		readinessRouter: options.RouterFactory.NewRouter(),
		internalRouter:  options.RouterFactory.NewRouter(),
		handlers:        options.Handlers,
		wrapHandler:     options.WrapHandler,
		versionBuilder:  options.VersionBuilder,
		stateReader:     options.ServiceStateReader,
		exitFunc:        options.ExitFunc,
		sendChan:        make(chan bool, 1),
		receiveChan:     make(chan bool, 1),
	}
}

// NewExitFunc returns a new exit function. It wraps the shutdownFunc and executed an os.exit after the shutdown is
// completed with a slight delay, giving the quit handler a chance to return a status.
func NewExitFunc(log Logger, shutdownFunc ShutdownFunc) func(int) {
	return func(code int) {
		log.Debug("ServiceExit", "Performing service exit")

		go func() {
			if shutdownFunc != nil {
				log.Debug("ShutdownFunc", "Calling shutdown func")
				shutdownFunc(log)
			}

			if code != 0 {
				time.Sleep(500 * time.Millisecond)
			}

			log.Debug("ServiceExit", "Calling os.Exit(%v)", code)
			os.Exit(code)
		}()

		// Allow the go-routine to be spawned
		time.Sleep(1 * time.Millisecond)
	}
}

// NewServiceStateReader instantiates a new basic ServiceStateReader implementation, which always returns true
// for it's state methods.
func NewServiceStateReader() ServiceStateReader {
	return &serviceStateReaderImpl{}
}

/* ServiceStateReader implementation */

func (s *serviceStateReaderImpl) IsLive() bool {
	return true
}

func (s *serviceStateReaderImpl) IsReady() bool {
	return true
}

func (s *serviceStateReaderImpl) IsHealthy() bool {
	return true
}

/* ServiceOptions implementation */

// SetHandlers is used to update the handler references in ServiceOptions to use the correct middleware and state.
func (o *ServiceOptions) SetHandlers() {
	factory := NewServiceHandlerFactory(o.MiddlewareWrapper, o.VersionBuilder, o.ServiceStateReader, o.ExitFunc)
	o.Handlers = factory.NewHandlers()
	o.WrapHandler = factory
}

/* Service implementation */

func (s *serviceImpl) Run(ctx context.Context) {
	s.log.Info("Service", "%s: %s", s.globals.AppName, s.versionBuilder.ToString())

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-s.receiveChan:
			s.log.Debug("UnexpectedShutdownReceived", "Server shut down unexpectedly")
			// One of the servers has shut down unexpectedly. Because this makes the whole service unreliable, shutdown.
			break
		case <-ctx.Done():
			s.log.Debug("ServiceCancel", "Cancellation request received")

			// Shutdown any running http servers
			s.quitting = true
			s.sendChan <- true
			break
		case <-sigs:
			s.log.Debug("GracefulShutdown", "Handling Sigterm/SigInt")
			break
		}

		if !s.quitting {
			// Some other go-routine is already taking care of the shutdown
			s.quitting = true
			s.sendChan <- true
		}

		// Trigger graceful shutdown
		s.exitFunc(0)
		done <- true
	}()

	s.runReadinessServer()
	s.runInternalServer()
	s.runPublicServer()

	<-done // Wait for our shutdown

	// since service.ExitFunc calls os.Exit(), we'll never get here
}

func (s *serviceImpl) AddRoute(name string, routes []string, methods []string, middlewares []Middleware, handler Handle) {
	s.addRoute(s.publicRouter, publicSubsystem, name, routes, methods, middlewares, handler)
}

func (s *serviceImpl) addRoute(router *Router, subsystem, name string, routes []string, methods []string, middlewares []Middleware, handler Handle) {
	for _, path := range routes {
		wrappedHandler := s.wrapHandler.Wrap(subsystem, name, middlewares, handler)

		for _, method := range methods {
			router.Router.Handle(method, path, wrappedHandler)
		}
	}
}

func (s *serviceImpl) runHTTPServer(port int, router *Router) {
	addr := fmt.Sprintf(":%v", port)
	svr := &http.Server{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
		Addr:         addr,
		Handler:      router.Router,
	}

	go func() {
		// Blocking until the server stops.
		svr.ListenAndServe()

		// Notify the service that the server has stopped.
		s.receiveChan <- true
	}()

	go func() {
		// Monitor sender channel and close server on signal.
		select {
		case sig := <-s.sendChan:
			// Properly close the server if possible.
			if svr != nil {
				svr.Close()
				svr = nil
			}
			// Continue sending the message
			s.sendChan <- sig
			break
		}
	}()
}

// RunReadinessServer runs the readiness service as a go-routine
func (s *serviceImpl) runReadinessServer() {
	const subsystem = "readiness"

	router := s.readinessRouter

	s.addRoute(router, subsystem, "root", []string{"/"}, MethodsForGet, DefaultMiddlewares, s.handlers.RootHandler.NewRootHandler())
	s.addRoute(router, subsystem, "liveness", []string{"/service/liveness"}, MethodsForGet, DefaultMiddlewares, s.handlers.LivenessHandler.NewLivenessHandler())
	s.addRoute(router, subsystem, "readiness", []string{"/service/readiness"}, MethodsForGet, DefaultMiddlewares, s.handlers.ReadinessHandler.NewReadinessHandler())

	s.log.Info("RunReadinessServer", "%s %s running on localhost:%d.", s.globals.AppName, subsystem, s.readinessPort)

	s.runHTTPServer(s.readinessPort, router)
}

// RunInternalServer runs the internal service as a go-routine
func (s *serviceImpl) runInternalServer() {
	const subsystem = "internal"

	router := s.internalRouter

	s.addRoute(router, subsystem, "root", []string{"/"}, MethodsForGet, DefaultMiddlewares, s.handlers.RootHandler.NewRootHandler())
	s.addRoute(router, subsystem, "health_check", []string{"/health_check", "/healthz"}, MethodsForGet, DefaultMiddlewares, s.handlers.HealthHandler.NewHealthHandler())
	s.addRoute(router, subsystem, "metrics", []string{"/metrics"}, MethodsForGet, DefaultMiddlewares, s.handlers.MetricsHandler.NewMetricsHandler())
	s.addRoute(router, subsystem, "quit", []string{"/quit"}, MethodsForGet, DefaultMiddlewares, s.handlers.QuitHandler.NewQuitHandler())

	s.log.Info("RunInternalServer", "%s %s running on localhost:%d.", s.globals.AppName, subsystem, s.internalPort)

	s.runHTTPServer(s.internalPort, router)
}

// RunPublicServer runs the public service on the current thread.
func (s *serviceImpl) runPublicServer() {
	router := s.publicRouter

	s.addRoute(router, publicSubsystem, "root", []string{"/"}, MethodsForGet, DefaultMiddlewares, s.handlers.RootHandler.NewRootHandler())
	s.addRoute(router, publicSubsystem, "version", []string{"/service/version"}, MethodsForGet, DefaultMiddlewares, s.handlers.VersionHandler.NewVersionHandler())
	s.addRoute(router, publicSubsystem, "liveness", []string{"/service/liveness"}, MethodsForGet, DefaultMiddlewares, s.handlers.LivenessHandler.NewLivenessHandler())
	s.addRoute(router, publicSubsystem, "readiness", []string{"/service/readiness"}, MethodsForGet, DefaultMiddlewares, s.handlers.ReadinessHandler.NewReadinessHandler())

	s.log.Info("RunPublicService", "%s %s running on localhost:%d.", s.globals.AppName, publicSubsystem, s.port)

	s.runHTTPServer(s.port, router)
}
