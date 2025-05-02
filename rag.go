package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	vstore, err := NewWeaviateOllamaVectorStore()
	if err != nil {
		return nil, fmt.Errorf("failed to define indexer and retriever: %w", err)
	}
	log.Println(vstore.String(), "initialized")

	g, err := genkit.Init(ctx,
		genkit.WithPlugins(
			&googlegenai.GoogleAI{},
		),
		genkit.WithDefaultModel("googleai/gemini-2.0-flash"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Genkit: %w", err)
	}

	rag := &LottoRAGAI{}

	indexFlow := genkit.DefineFlow(
		g, "indexWinningHistoryFlow",
		func(ctx context.Context, w *Winning) (any, error) {
			rag.mu.Lock()
			defer rag.mu.Unlock()

			nums := append(w.Numbers, w.Bonus)
			b, err := json.Marshal(nums)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal winning: %w", err)
			}
			text := string(b)
			textID := ragkit.GenerateID(text)
			if exists, err := vstore.Exists(ctx, textID); err != nil {
				return nil, fmt.Errorf("failed to check if winning exists: %w", err)
			} else if exists {
				log.Println("winning already exists", textID)
				return nil, nil
			}

			doc := ragkit.Document{
				ID:   textID,
				Text: text,
				Metadata: map[string]any{
					"issue_no":     w.IssueNo,
					"first_prize":  w.FirstPrize,
					"first_count":  w.FirstCount,
					"second_prize": w.SecondPrize,
					"second_count": w.SecondCount,
				},
			}
			_, err = vstore.Index(ctx, doc)
			if err != nil {
				return nil, fmt.Errorf("failed to index winning: %w", err)
			}

			return nil, nil
		},
	)

	pickNumFlow := genkit.DefineFlow(
		g, "pickLuckyNumsFlow",
		func(ctx context.Context, cnt int) (Lucky, error) {
			rag.mu.Lock()
			defer rag.mu.Unlock()

			luckyNums := generateLottoNumbers(7)

			b, err := json.Marshal(luckyNums)
			if err != nil {
				return Lucky{}, fmt.Errorf("failed to marshal luckyNums: %w", err)
			}

			retrieves, err := vstore.RetrieveText(ctx, string(b), cnt+50, "issue_no")
			if err != nil {
				return Lucky{}, fmt.Errorf("failed to retrieve winning: %w", err)
			}

			// log.Println("retrieved", len(retrieves))
			// for _, r := range retrieves {
			// 	log.Println(r.Text, r.Metadata)
			// }

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
			rag.mu.Lock()
			defer rag.mu.Unlock()

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
- 아무 생각이 떠오르지 않으면 /smallchat 행동을 선택한다.

주의사항:
- 행동 이름은 반드시 / 로 시작하는 소문자 단어로 출력한다.
`
			userPromptFmt := "다음은 사용자의 채팅 메시지. 이 메시지를 분석해서 적절한 행동을 선택해:\n%s"

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

	rag.IndexWinningHistoryFlow = indexFlow
	rag.PickLuckyNumsFlow = pickNumFlow
	rag.ChatbotFlow = chatbotFlow

	return rag, nil
}
