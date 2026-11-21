// Command sendbox-repo serves the package repository (index + *.pkg.tar.gz)
// over HTTP. It replaces the former Python http.server based serve.py.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8000", "listen address")
	dir := flag.String("dir", ".", "directory to serve")
	flag.Parse()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           http.FileServer(http.Dir(*dir)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()

	log.Println("serving", *dir, "at", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Println(err)
		os.Exit(1)
	}
}
