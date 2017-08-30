package model

import "time"

type (
	ShutdownFunc func(log Logger)

	ServiceOptions struct {
		Port                  int
		ReadinessPort         int
		InternalPort          int
		Logger                Logger
		Metrics               Metrics
		RouterFactory         RouterFactory
		ServiceHandlerFactory ServiceHandlerFactory
		VersionBuilder        VersionBuilder
		ShutdownFunc          ShutdownFunc
		ExitFunc              ExitFunc
		ServerTimeout         time.Duration
	}

	Service interface {
		Run()
		AddRoute(name string, routes []string, methods []string, middlewares []Middleware, handler Handle)
	}
)
