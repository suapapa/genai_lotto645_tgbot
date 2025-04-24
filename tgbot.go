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
	ai       *LottoAI
	b        *telego.Bot
	UpdateCh <-chan telego.Update
	cancelF  context.CancelFunc
	chatIDs  []int64
}

func NewTelegramBot(lottoAI *LottoAI, apiToken string, chatIDs ...int64) (*TelegramBot, error) {
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
	tb.cancelF()
	if err := tb.b.Close(context.Background()); err != nil {
		fmt.Printf("failed to close bot: %v", err)
	}
}

func (tb *TelegramBot) Listen() {
	for update := range tb.UpdateCh {
		if update.Message == nil {
			continue
		}

		var slashCmd string
		var slashArgs []string
		if strings.HasPrefix(update.Message.Text, "/") {
			slashCmd = update.Message.Text
			slashArgs = strings.Split(slashCmd, " ")
			if len(slashArgs) > 1 {
				slashCmd = slashArgs[0]
				slashArgs = slashArgs[1:]
			} else if len(slashArgs) == 1 {
				slashCmd = slashArgs[0]
				slashArgs = nil
			}
		} else {
			slashCmd = update.Message.Text
			slashArgs = nil
		}

		id := update.Message.Chat.ID
		switch slashCmd {
		case "/start":
			tb.sendMessage(update.Message.Chat.ID, "pong")
		case "/addme":
			// check if already added
			for _, chatID := range tb.chatIDs {
				if chatID == id {
					tb.sendMessage(id, "이미 등록되었습니다.")
					continue
				}
			}
			tb.chatIDs = append(tb.chatIDs, id)
			tb.sendMessage(id, "지금부터 매 주 로또 번호를 보내드리겠습니다.")
		case "/ai", "/ailotto":
			tb.sendMessage(id, "초지능의 도움을 받아 로또 번호를 생성합니다...")
			nums, err := tb.ai.PickNumFlow.Run(context.Background(), struct{}{})
			if err != nil {
				tb.sendMessage(id, fmt.Sprintf("로또 번호 생성 실패: %v", err))
				continue
			}
			numStrs := make([]string, len(nums))
			for i, num := range nums {
				numStrs[i] = fmt.Sprintf("%d", num)
			}
			tb.sendMessage(id, fmt.Sprintf("생성완료 : %s", strings.Join(numStrs, ", ")))

		case "/lotto":
			if len(slashArgs) > 0 {
				cntStr := slashArgs[0]
				cnt, err := strconv.Atoi(cntStr)
				if err != nil {
					log.Printf("'%v %v'", cntStr, slashArgs)
					tb.sendMessage(id, "로또 번호를 생성할 개수를 입력하세요.")
					continue
				}
				if cnt > 50 {
					tb.sendMessage(id, "한 번에 최대 50개까지 생성할 수 있습니다.")
					cnt = 50
				}

				tb.sendMessage(id, fmt.Sprintf("로또 번호 %d 개를 생성합니다...", cnt))
				var result []string
				for i := 0; i < int(cnt); i++ {
					nums := generateLottoNumbers(6)
					numStrs := make([]string, len(nums))
					for j, num := range nums {
						numStrs[j] = fmt.Sprintf("%d", num)
					}
					result = append(result, strings.Join(numStrs, ", "))
					if (i+1)%5 == 0 || i == cnt-1 {
						tb.sendMessage(id, strings.Join(result, "\n"))
						result = nil
					}
				}
				tb.sendMessage(id, "생성완료")
			} else {
				tb.sendMessage(id, "로또 번호를 생성합니다...")
				nums := generateLottoNumbers(6)
				numStrs := make([]string, len(nums))
				for i, num := range nums {
					numStrs[i] = fmt.Sprintf("%d", num)
				}
				tb.sendMessage(id, fmt.Sprintf("생성완료 : %s", strings.Join(numStrs, ", ")))
			}
		default:
			tb.sendMessage(id, fmt.Sprintf("몰-루 : '%s'", update.Message.Text))
		}
	}
}

func (tb *TelegramBot) sendMessage(id int64, text string) {
	cid := telego.ChatID{ID: id}
	msg := telegoutil.Message(cid, text)
	if _, err := tb.b.SendMessage(context.Background(), msg); err != nil {
		fmt.Printf("failed to send message: %v", err)
	}
}
