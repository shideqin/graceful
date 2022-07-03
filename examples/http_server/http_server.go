package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/shideqin/graceful"
)

func main() {
	// start http server
	ln, err := graceful.ListenTCP("tcp", ":80")
	if err != nil {
		log.Fatalf("graceful.ListenTCP error: %v", err)
	}
	server := &http.Server{Handler: router()}
	go func() {
		err = server.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("server.Serve error: %v\n", err)
		}
	}()

	// graceful
	graceful.HandleSignal(func(ctx context.Context) {
		err = server.Shutdown(ctx)
		if err != nil {
			log.Printf("server.Shutdown error: %v\n", err)
		}
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprintf(w, `{"health":true}`)
}

func router() *mux.Router {
	//设置访问的路由
	r := mux.NewRouter()
	r.Use(func(h http.Handler) http.Handler {
		return handlers.LoggingHandler(os.Stdout, h)
	})
	r.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	r.HandleFunc("/health", health).Methods("GET")
	return r
}
