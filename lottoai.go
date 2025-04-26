package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
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

type Lucky struct {
	Picks [][]int `json:"picks" yaml:"picks"`
}

type LottoRAGAI struct {
	IndexWinningHistoryFlow *core.Flow[*Winning, any, struct{}]
	PickLuckyNumsFlow       *core.Flow[int, Lucky, struct{}]
	ChatbotFlow             *core.Flow[string, string, struct{}] // Input: user message, Output: bot action (rand, ai, hello, help)
}

func NewLottoRAGAI(
	retrievePrompt string,
	systemPrompt string,
	userPromptFmt string,
) (*LottoRAGAI, error) {
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

	winHistoryIndexer, winHistoryRetriver, err := weaviate.DefineIndexerAndRetriever(ctx, g, weaviate.ClassConfig{
		Class:    "WinningHistory",
		Embedder: o.DefineEmbedder(g, o.ServerAddress, embedderModelName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to define indexer and retriever: %w", err)
	}

	indexFlow := genkit.DefineFlow(
		g, "indexWinningHistoryFlow",
		func(ctx context.Context, w *Winning) (any, error) {
			b, err := yaml.Marshal(w)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal winning: %w", err)
			}
			// log.Println("indexing", string(b))
			doc := ai.DocumentFromText(string(b), nil)
			err = ai.Index(ctx, winHistoryIndexer,
				ai.WithDocs(doc),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to index winning: %w", err)
			}
			return nil, nil
		},
	)

	pickNumFlow := genkit.DefineFlow(
		g, "pickLuckyNumsFlow",
		func(ctx context.Context, cnt int) (Lucky, error) {
			resp, err := ai.Retrieve(ctx, winHistoryRetriver, ai.WithTextDocs(retrievePrompt))
			if err != nil {
				return Lucky{}, fmt.Errorf("failed to retrive winning: %w", err)
			}

			s, _, err := genkit.GenerateData[Lucky](
				ctx, g,
				ai.WithDocs(resp.Documents...),
				ai.WithSystem(systemPrompt),
				ai.WithPrompt(fmt.Sprintf(userPromptFmt, cnt)),
			)
			if err != nil {
				return *s, fmt.Errorf("failed to generate winning: %w", err)
			}
			if len((*s).Picks) != cnt {
				return *s, fmt.Errorf("invalid winning numbers: %v", s)
			}
			// sort.Ints(*s)
			return *s, nil
		},
	)

	ret := &LottoRAGAI{
		IndexWinningHistoryFlow: indexFlow,
		PickLuckyNumsFlow:       pickNumFlow,
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
