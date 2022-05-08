package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/taiidani/deploy/internal"
)

const defaultBind = ":8082"

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
		Addr:    defaultBind,
		Handler: mux,
	}

	fmt.Println("Server started at", defaultBind)
	go srv.ListenAndServe()
	<-ctx.Done()

	fmt.Println("Server shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	return srv.Shutdown(shutdown)
}
