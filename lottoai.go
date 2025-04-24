package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/ollama"
	"github.com/firebase/genkit/go/plugins/weaviate"
	"github.com/goccy/go-yaml"
)

var (
	ollamaAddr        = cmp.Or(os.Getenv("OLLAMA_ADDR"), "http://localhost:11434")
	weaviateAddr      = cmp.Or(os.Getenv("WV_ADDR"), "http://localhost:9035")
	embedderModelName = "bge-m3"
)

type LottoAI struct {
	// g           *genkit.Genkit
	// wv          *weaviate.Weaviate
	// o           *ollama.Ollama
	// indexer     ai.Indexer
	// retriver    ai.Retriever
	IndexFlow   *core.Flow[*Winning, any, struct{}]
	PickNumFlow *core.Flow[struct{}, []int, struct{}]
}

func NewLottoAI(
	retrievePrompt string,
	systemPrompt string,
	userPrompt string,
) (*LottoAI, error) {
	ctx := context.Background()

	wvSchema, wvAddr := toSchemaAndAddr(weaviateAddr)
	wv := &weaviate.Weaviate{
		Scheme: wvSchema,
		Addr:   wvAddr,
	}

	o := &ollama.Ollama{
		ServerAddress: ollamaAddr,
	}

	// Initialize a Genkit instance.
	g, err := genkit.Init(ctx,
		// Install the Google AI plugin which provides Gemini models.
		genkit.WithPlugins(
			&googlegenai.GoogleAI{},
			// &googlegenai.VertexAI{},
			o,
			wv,
		),
		// Set the default model to use for generate calls.
		genkit.WithDefaultModel("googleai/gemini-2.0-flash"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Genkit: %w", err)
	}

	indexer, retriver, err := weaviate.DefineIndexerAndRetriever(ctx, g, weaviate.ClassConfig{
		Class:    "WinningHistory",
		Embedder: o.DefineEmbedder(g, o.ServerAddress, embedderModelName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to define indexer and retriever: %w", err)
	}

	indexFlow := genkit.DefineFlow(
		g, "indexWinningFlow",
		func(ctx context.Context, w *Winning) (any, error) {
			b, err := yaml.Marshal(w)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal winning: %w", err)
			}
			// log.Println("indexing", string(b))
			doc := ai.DocumentFromText(string(b), nil)
			err = ai.Index(ctx, indexer,
				ai.WithDocs(doc),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to index winning: %w", err)
			}
			return nil, nil
		},
	)

	pickNumFlow := genkit.DefineFlow(
		g, "pickNumFlow",
		func(ctx context.Context, _ struct{}) ([]int, error) {
			resp, err := ai.Retrieve(ctx, retriver, ai.WithTextDocs(retrievePrompt))
			if err != nil {
				return nil, fmt.Errorf("failed to retrive winning: %w", err)
			}

			s, _, err := genkit.GenerateData[[]int](
				ctx, g,
				ai.WithDocs(resp.Documents...),
				ai.WithSystem(systemPrompt),
				ai.WithPrompt(userPrompt),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to generate winning: %w", err)
			}
			if len(*s) != 6 {
				return nil, fmt.Errorf("invalid winning numbers: %v", s)
			}
			sort.Ints(*s)
			return *s, nil
		},
	)

	ret := &LottoAI{
		IndexFlow:   indexFlow,
		PickNumFlow: pickNumFlow,
	}

	return ret, nil
}

func toSchemaAndAddr(url string) (string, string) {
	parts := strings.Split(url, "://")
	if len(parts) != 2 {
		return "", ""
	}
	schema := parts[0]
	addr := parts[1]

	return schema, addr
}
