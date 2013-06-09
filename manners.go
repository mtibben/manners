package manners

import (
	"net"
	"net/http"
	"os"
	"sync"
)

var (
	ShutdownChannel = make(chan os.Signal)
	shutdownHandler func()
	waitGroup       = sync.WaitGroup{}
)

func BePolite(srv *http.Server) *GracefulServer {
	return &GracefulServer{
		delegate: srv,
	}
}

type GracefulServer struct {
	delegate *http.Server
}

func (srv *GracefulServer) ListenAndServe() error {

	addr := srv.delegate.Addr
	if addr == "" {
		addr = ":http"
	}
	l, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}

	gracefulListener := NewListener(l)

	return srv.Serve(gracefulListener)
}

func (srv *GracefulServer) Serve(listener *GracefulListener) error {
	listener.CloseOnShutdown()
	go WaitForShutdown()

	err := srv.delegate.Serve(listener)
	if err == nil {
		return nil
	} else if _, ok := err.(mannersError); ok {
		return nil
	}
	return err
}

func ListenAndServe(addr string, handler http.Handler) error {
	server := BePolite(&http.Server{Addr: addr, Handler: handler})
	return server.ListenAndServe()
}

func Serve(listener *GracefulListener, handler http.Handler) error {
	srv := BePolite(&http.Server{Handler: handler})
	return srv.Serve(listener)
}

func RunRoutine(f func()) {
	StartRoutine()
	go func() {
		defer FinishRoutine()
		f()
	}()
}

func StartRoutine() {
	waitGroup.Add(1)
}

func FinishRoutine() {
	waitGroup.Done()
}

func WaitForShutdown() {
	<-ShutdownChannel
	shutdownHandler()
}

func NewListener(oldListener net.Listener) *GracefulListener {
	listener := GracefulListener{oldListener, true}
	return &listener
}

type GracefulListener struct {
	net.Listener
	open bool
}

func (this *GracefulListener) Accept() (net.Conn, error) {
	conn, err := this.Listener.Accept()
	if err != nil {
		if !this.open {
			waitGroup.Wait()
			err = mannersError{err}
		}
		return nil, err
	}
	StartRoutine()
	return GracefulConnection{conn}, nil
}

func (this *GracefulListener) Close() error {
	if !this.open {
		return nil
	}
	this.open = false
	err := this.Listener.Close()
	return err
}

func (this *GracefulListener) CloseOnShutdown() {
	shutdownHandler = func() { this.Close() }
}

type GracefulConnection struct {
	net.Conn
}

func (this GracefulConnection) Close() error {
	err := this.Conn.Close()
	FinishRoutine()
	return err
}

type mannersError struct {
	error
}
