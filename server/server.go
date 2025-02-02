package server

import (
	"context"
	"github.com/sharat87/httpbun/exchange"
	"github.com/sharat87/httpbun/routes"
	"github.com/sharat87/httpbun/server/spec"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Server struct {
	*http.Server
	spec    spec.Spec
	routes  []routes.Route
	closeCh chan error
}

func StartNew(spec spec.Spec) Server {
	sslCertFile := os.Getenv("HTTPBUN_SSL_CERT")
	sslKeyFile := os.Getenv("HTTPBUN_SSL_KEY")

	server := Server{
		Server: &http.Server{
			Addr: spec.BindTarget,
		},
		spec:    spec,
		routes:  routes.GetRoutes(),
		closeCh: make(chan error, 1),
	}
	server.Handler = server

	listener, err := net.Listen("tcp", spec.BindTarget)
	if err != nil {
		log.Fatalf("Error listening on %q: %v", spec.BindTarget, err)
	}

	go func() {
		defer close(server.closeCh)
		if sslCertFile == "" {
			server.closeCh <- server.Serve(listener)
		} else {
			server.closeCh <- server.ServeTLS(listener, sslCertFile, sslKeyFile)
		}
	}()

	return server
}

func (s Server) Wait() error {
	return <-s.closeCh
}

func (s Server) CloseAndWait() {
	if s.Server != nil {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFunc()
		if err := s.Server.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down server: %v", err)
			if err := s.Server.Close(); err != nil {
				log.Printf("Error closing server: %v", err)
			}
		}
	}
	log.Print(s.Wait())
}

func (s Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, s.spec.PathPrefix) {
		http.NotFound(w, req)
		return
	}

	ex := exchange.New(w, req, s.spec)

	incomingIP := ex.FindIncomingIPAddress()
	log.Printf(
		"From ip=%s %s %s%s",
		incomingIP,
		req.Method,
		req.Host,
		req.URL.String(),
	)

	for _, route := range s.routes {
		if ex.MatchAndLoadFields(route.Pat) {
			route.Fn(ex)
			return
		}
	}

	log.Printf("NotFound ip=%s %s %s", incomingIP, req.Method, req.URL.String())
	http.NotFound(w, req)
}
