package main

import (
	"cmp"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	ollama_api "github.com/ollama/ollama/api"
	ollama_embedder "github.com/suapapa/go_ragkit/embedder/ollama"
	"github.com/weaviate/weaviate-go-client/v4/weaviate"

	ragkit "github.com/suapapa/go_ragkit"
	weaviate_vstore "github.com/suapapa/go_ragkit/vector_store/weaviate"
)

var (
	ollamaAddr       = cmp.Or(os.Getenv("OLLAMA_ADDR"), "http://localhost:11434")
	ollamaEmbedModel = cmp.Or(os.Getenv("OLLAMA_EMBED_MODEL"), "bge-m3:latest")
	weaviateAddr     = cmp.Or(os.Getenv("WEAVIATE_ADDR"), "http://localhost:8080")

	vectorDBClassName = "lottary_win_history" // -> LottaryWinHistory
)

func NewWeaviateOllamaVectorStore() (ragkit.VectorStore, error) {
	// initialize ollama
	ollamaURL, err := url.Parse(ollamaAddr)
	if err != nil {
		return nil, err
	}
	ollamaClient := ollama_api.NewClient(ollamaURL, http.DefaultClient)
	embedder := ollama_embedder.New(ollamaClient, ollamaEmbedModel)

	weaviateURL, err := url.Parse(weaviateAddr)
	if err != nil {
		return nil, err
	}

	// initialize weaviate
	weaviateClient, err := weaviate.NewClient(weaviate.Config{
		Host:           weaviateURL.Host,
		Scheme:         weaviateURL.Scheme,
		StartupTimeout: 3 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	// define vector_store
	log.Println("defining vector_store")
	return weaviate_vstore.New(weaviateClient, vectorDBClassName, embedder), nil
}
