package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/taiidani/deploy/internal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context) error {
	mux := http.NewServeMux()
	gh := internal.NewGitHubClient()
	if err := gh.Serve(mux); err != nil {
		return err
	}

	srv := http.Server{
		Addr:    ":8082",
		Handler: mux,
	}

	go srv.ListenAndServe()
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	return srv.Shutdown(shutdown)
}
