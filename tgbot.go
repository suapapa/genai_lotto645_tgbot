package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

type TelegramBot struct {
	ai       *LottoRAGAI
	b        *telego.Bot
	UpdateCh <-chan telego.Update
	cancelF  context.CancelFunc
	chatIDs  []int64

	aiGenCnt   uint64
	randGenCnt uint64
}

func NewTelegramBot(lottoAI *LottoRAGAI, apiToken string, chatIDs ...int64) (*TelegramBot, error) {
	b, err := telego.NewBot(apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	ctx, cancelF := context.WithCancel(context.Background())
	tuCh, err := b.UpdatesViaLongPolling(ctx, nil) // 폴링방식으로
	if err != nil {
		cancelF()
		return nil, fmt.Errorf("failed to get updates: %w", err)
	}

	return &TelegramBot{
		ai:       lottoAI,
		b:        b,
		UpdateCh: tuCh,
		cancelF:  cancelF,
		chatIDs:  chatIDs,
	}, nil
}

func (tb *TelegramBot) Close() {
	if err := tb.b.Close(context.Background()); err != nil {
		fmt.Printf("failed to close bot: %v", err)
	}
	tb.cancelF()
}

func (tb *TelegramBot) Listen() {
	for update := range tb.UpdateCh {
		if update.Message == nil {
			continue
		}

		go func() {
			var prompt string
			var slashArgs []string
			if strings.HasPrefix(update.Message.Text, "/") {
				slashArgs = strings.Split(update.Message.Text, " ")
				if len(slashArgs) > 1 {
					prompt = slashArgs[0]
					slashArgs = slashArgs[1:]
				} else if len(slashArgs) == 1 {
					prompt = slashArgs[0]
				}
			} else {
				prompt = update.Message.Text
			}

			id := update.Message.Chat.ID
			if strings.HasPrefix(prompt, "/") {
				if err := tb.Do(id, prompt, slashArgs); err != nil {
					tb.sendMessage(id, fmt.Sprintf("Error: %v", err))
				}
			} else {
				log.Printf("thinking prompt: %s...", prompt)
				cmd, err := tb.ai.ChatbotFlow.Run(context.Background(), prompt)
				if err != nil {
					tb.sendMessage(id, fmt.Sprintf("Error: %v", err))
				}
				log.Printf("got cmd: %+v", cmd)

				if err := tb.Do(id, cmd.Action, cmd.Args); err != nil {
					tb.sendMessage(id, fmt.Sprintf("Error: %v", err))
				}
			}
		}()
	}
}

func (tb *TelegramBot) Do(id int64, slashCmd string, slashArgs []string) error {
	switch slashCmd {
	case "/start":
		log.Printf("start cmd received from %d", id)
		// tb.sendMessage(update.Message.Chat.ID, "pong")
	case "/ai", "/ailotto":
		var cnt int
		if len(slashArgs) > 0 {
			var err error
			cntStr := slashArgs[0]
			cnt, err = strconv.Atoi(cntStr)
			if err != nil {
				return fmt.Errorf("invalid count: %v", err)
			}
		} else {
			cnt = 1
		}

		if cnt > 50 {
			tb.sendMessage(id, "한 번에 최대 50개까지 생성할 수 있습니다.")
			cnt = 50
		}

		tb.sendMessage(id, fmt.Sprintf("초지능의 힘으로 로또 번호 %d 개를 생성합니다...", cnt))
		lucky, err := tb.ai.PickLuckyNumsFlow.Run(context.Background(), cnt)
		if err != nil {
			return fmt.Errorf("failed to generate lucky numbers: %v", err)
		}

		// show 5 numbers at a time
		var result []string
		for i, nums := range lucky.Picks {
			numStrs := make([]string, len(nums))
			for j, num := range nums {
				numStrs[j] = fmt.Sprintf("%02d", num)
			}
			result = append(result, strings.Join(numStrs, ", "))
			if (i+1)%5 == 0 || i == cnt-1 {
				tb.sendMessage(id, strings.Join(result, "\n"))
				result = nil
			}
		}
		tb.aiGenCnt += uint64(cnt)
		log.Printf("%d 의 요청으로 %d 개의 로또 번호를 초지능으로 생성했습니다.", id, cnt)
		tb.sendMessage(id, "생성완료")

	case "/rand", "/lotto":
		var cnt int
		if len(slashArgs) > 0 {
			var err error
			cntStr := slashArgs[0]
			cnt, err = strconv.Atoi(cntStr)
			if err != nil {
				return fmt.Errorf("invalid count: %v", err)
			}
		} else {
			cnt = 1
		}

		tb.sendMessage(id, fmt.Sprintf("로또 번호 %d 개를 생성합니다...", cnt))
		var result []string
		for i := 0; i < int(cnt); i++ {
			nums := generateLottoNumbers(6)
			numStrs := make([]string, len(nums))
			for j, num := range nums {
				numStrs[j] = fmt.Sprintf("%02d", num)
			}
			result = append(result, strings.Join(numStrs, ", "))
			if (i+1)%5 == 0 || i == cnt-1 {
				tb.sendMessage(id, strings.Join(result, "\n"))
				result = nil
			}
		}
		tb.randGenCnt += uint64(cnt)
		log.Printf("%d 의 요청으로 %d 개의 로또 번호를 생성했습니다.", id, cnt)
		tb.sendMessage(id, "생성완료")

	case "/hello":
		var reply string
		if len(slashArgs) > 0 {
			reply = strings.Join(slashArgs, "\n")
		} else {
			reply = "안녕하세요. 저는 김점지 봇입니다.\n행운의 로또 번호를 생성해드릴게요."
		}
		tb.sendMessage(id, reply)

	case "/stat":
		tb.sendMessage(id, fmt.Sprintf("AI 생성 횟수: %d\n랜덤 생성 횟수: %d", tb.aiGenCnt, tb.randGenCnt))

	case "/help":
		tb.sendMessage(id, "Usage:\n/ai [count] - AI 로또 번호 생성\n/rand [count] - 랜덤 로또 번호 생성\n/stat - 통계 확인")

	case "/credit":
		tb.sendMessage(id, `Credit:
내 이름은 김점지 봇.
Go 언어로 작성된 RAG 기반의 로또 번호 생성 봇이야.
내가 만들어진 내용은 아래의 블로그 링크에 있어.
https://homin.dev/blog/post/20250425_genai_lotto_tg_bot/`)

	default:
		tb.sendMessage(id, fmt.Sprintf("Unknown command: %s", slashCmd))
	}
	return nil
}

func (tb *TelegramBot) sendMessage(id int64, text string) {
	cid := telego.ChatID{ID: id}
	msg := telegoutil.Message(cid, text)
	if _, err := tb.b.SendMessage(context.Background(), msg); err != nil {
		fmt.Printf("failed to send message: %v", err)
	}
}
