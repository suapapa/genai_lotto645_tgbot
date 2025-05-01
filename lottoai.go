package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	ragkit "github.com/suapapa/go_ragkit"
)

type Lucky struct {
	Picks [][]int `json:"picks" yaml:"picks"`
}

type LottoRAGAI struct {
	vectorEmbedder ragkit.Vectorizer

	IndexWinningHistoryFlow *core.Flow[*Winning, any, struct{}]
	PickLuckyNumsFlow       *core.Flow[int, Lucky, struct{}]
	ChatbotFlow             *core.Flow[string, Cmd, struct{}] // Input: user message, Output: bot action (rand, ai, smallchat, help)

	mu sync.Mutex
}

type Cmd struct {
	Action string   `json:"action" yaml:"action"`
	Args   []string `json:"args" yaml:"args"`
}

func NewLottoRAGAI(
	retrievePrompt string,
	systemPrompt string,
	userPromptFmt string,
) (*LottoRAGAI, error) {
	ctx := context.Background()

	// Initialize a Genkit instance.
	g, err := genkit.Init(ctx,
		// Install the Google AI plugin which provides Gemini models.
		genkit.WithPlugins(
			&googlegenai.GoogleAI{},
			// &googlegenai.VertexAI{},
		),
		// Set the default model to use for generate calls.
		genkit.WithDefaultModel("googleai/gemini-2.0-flash"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Genkit: %w", err)
	}

	// winHistoryIndexer, winHistoryRetriver, err := weaviate.DefineIndexerAndRetriever(ctx, g, weaviate.ClassConfig{
	// 	Class:    "WinningHistory",
	// 	Embedder: o.DefineEmbedder(g, o.ServerAddress, embedderModelName),
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to define indexer and retriever: %w", err)
	// }
	vectorizer, err := NewWeaviateVectorizer()
	if err != nil {
		return nil, fmt.Errorf("failed to define indexer and retriever: %w", err)
	}

	ret := &LottoRAGAI{
		vectorEmbedder: vectorizer,
	}

	indexFlow := genkit.DefineFlow(
		g, "indexWinningHistoryFlow",
		func(ctx context.Context, w *Winning) (any, error) {
			ret.mu.Lock()
			defer ret.mu.Unlock()

			// b, err := yaml.Marshal(w)
			// if err != nil {
			// 	return nil, fmt.Errorf("failed to marshal winning: %w", err)
			// }
			// log.Println("indexing", string(b))
			// doc := ai.DocumentFromText(string(b), nil)
			// err = ai.Index(ctx, winHistoryIndexer,
			// 	ai.WithDocs(doc),
			// )
			// if err != nil {
			// 	return nil, fmt.Errorf("failed to index winning: %w", err)
			// }

			doc := ragkit.Document{
				Text: fmt.Sprintf("%v + %d", w.Numbers, w.Bonus),
				Metadata: map[string]any{
					"issue_no":     w.IssueNo,
					"first_prize":  w.FirstPrize,
					"first_count":  w.FirstCount,
					"second_prize": w.SecondPrize,
					"second_count": w.SecondCount,
				},
			}
			_, err = ret.vectorEmbedder.Index(ctx, doc)
			if err != nil {
				return nil, fmt.Errorf("failed to index winning: %w", err)
			}

			return nil, nil
		},
	)

	pickNumFlow := genkit.DefineFlow(
		g, "pickLuckyNumsFlow",
		func(ctx context.Context, cnt int) (Lucky, error) {
			ret.mu.Lock()
			defer ret.mu.Unlock()

			// resp, err := ai.Retrieve(ctx, winHistoryRetriver, ai.WithTextDocs(retrievePrompt))
			// if err != nil {
			// 	return Lucky{}, fmt.Errorf("failed to retrive winning: %w", err)
			// }

			retrieves, err := ret.vectorEmbedder.RetrieveText(ctx, retrievePrompt, 50)
			if err != nil {
				return Lucky{}, fmt.Errorf("failed to retrieve winning: %w", err)
			}

			docs := make([]*ai.Document, len(retrieves))
			for i, r := range retrieves {
				docs[i] = &ai.Document{
					Content: []*ai.Part{
						ai.NewTextPart(r.Text),
					},
					Metadata: r.Metadata,
				}
			}

			s, _, err := genkit.GenerateData[Lucky](
				ctx, g,
				ai.WithDocs(docs...),
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

	chatbotFlow := genkit.DefineFlow(
		g, "chatbotFlow",
		func(ctx context.Context, msg string) (Cmd, error) {
			ret.mu.Lock()
			defer ret.mu.Unlock()

			systemPrompt := `너의 이름은 김점지야.
너의 임무는 사용자의 채팅 메시지를 분석하여 적절한 행동(Action)을 선택하는 것이야.

- 사용자의 의도에 따라 **반드시 아래 중 하나의 행동을 출력**해야 해.
- 행동(Action) 목록:
  - /rand: 무작위(Random) 로또 번호를 추천해야 할 때
  - /ai: 인공지능(AI)을 이용해 로또 번호를 예측해야 할 때
  - /smallchat: 사용자와 소소한 대화를 할 때
  - /about: 봇의 정보를 출력해야 할 때

출력 규칙:
- action 필드에는 오직 행동 이름(/rand, /ai, /smallchat, /about)만 출력한다.
- /rand, /ai 행동에 대해서는 args 필드의 배열의 첫번째에 한 번에 몇개의 로또 번호를 출력할지 적어야 한다.
- /smallchat 행동에 대해서는 args 필드의 배열의 첫번째에 적절한 인사말을 적어야 한다.
- args 필드에는 추가적인 정보가 없다면 빈 배열을 출력한다.
- 다른 문장이나 설명은 절대 추가하지 않는다.
- 여러 행동이 떠오를 경우, 가장 먼저 떠오른 하나를 고른다.

주의사항:
- 행동 이름은 반드시 / 로 시작하는 소문자 단어로 출력한다.
`
			userPromptFmt := "다음은 사용자의 채팅 메시지야. 이 메시지를 분석해서 적절한 행동을 선택해줘:\n%s"

			s, _, err := genkit.GenerateData[Cmd](
				ctx, g,
				ai.WithSystem(systemPrompt),
				ai.WithPrompt(fmt.Sprintf(userPromptFmt, msg)),
			)
			if err != nil {
				return Cmd{}, fmt.Errorf("failed to generate winning: %w", err)
			}
			return *s, nil
		},
	)

	ret.IndexWinningHistoryFlow = indexFlow
	ret.PickLuckyNumsFlow = pickNumFlow
	ret.ChatbotFlow = chatbotFlow

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
