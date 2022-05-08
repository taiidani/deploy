package internal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/google/go-github/v44/github"
)

type GitHubClient struct {
	client  *github.Client
	hmacKey []byte
}

func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		hmacKey: []byte(os.Getenv("WEBHOOK_HMAC")),
	}
}

const (
	ErrCouldNotReadRequestBody = "Could not read request body"
	ErrCouldNotVerifyPayload   = "Could not verify payload"
	ErrCouldNotDecodeWebhook   = "Could not decode webhook body"
	ErrUnknownEvent            = "Unknown event"
)

func (g *GitHubClient) Serve(mux *http.ServeMux) error {
	mux.HandleFunc("/webhook", g.handleWebhook)
	return nil
}

func (g *GitHubClient) handleWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body := []byte{}
	if _, err := r.Body.Read(body); err != nil {
		http.Error(w, ErrCouldNotReadRequestBody, http.StatusBadRequest)
		log.Println(ErrCouldNotReadRequestBody+":", err)
		return
	}

	if ok := g.verifyHmac(r.Header.Get("X-Hub-Signature-256"), body); !ok {
		http.Error(w, ErrCouldNotVerifyPayload, http.StatusBadRequest)
		return
	}

	switch r.Header.Get("X-GitHub-Event") {
	case "deployment":
		payload := github.DeploymentEvent{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, ErrCouldNotDecodeWebhook, http.StatusBadRequest)
			log.Println(ErrCouldNotDecodeWebhook+":", err)
			return
		}

		nomad := NewNomadDeployer(g.client)
		go nomad.Deploy(context.Background(), &payload)
	default:
		http.Error(w, ErrUnknownEvent, http.StatusUnprocessableEntity)
		return
	}
}

func (g *GitHubClient) verifyHmac(signature string, message []byte) bool {
	// Calculate the signature of the given message
	mac := hmac.New(sha256.New, g.hmacKey)
	if _, err := mac.Write(message); err != nil {
		return false
	}
	expectedSignature := mac.Sum(nil)

	// Convert the incoming signature to bytes for comparison
	gotSignature, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	// Compare!
	return hmac.Equal(expectedSignature, gotSignature)
}
