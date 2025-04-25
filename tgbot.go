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

	aiGenCnt   uint64
	randGenCnt uint64
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

		username := update.Message.Chat.Username
		id := update.Message.Chat.ID
		switch slashCmd {
		case "/start":
			tb.sendMessage(update.Message.Chat.ID, "pong")
		// case "/addme":
		// 	// check if already added
		// 	for _, chatID := range tb.chatIDs {
		// 		if chatID == id {
		// 			tb.sendMessage(id, "이미 등록되었습니다.")
		// 			continue
		// 		}
		// 	}
		// 	tb.chatIDs = append(tb.chatIDs, id)
		// 	tb.sendMessage(id, "지금부터 매 주 로또 번호를 보내드리겠습니다.")
		case "/ai", "/ailotto":
			var cnt int
			if len(slashArgs) > 0 {
				var err error
				cntStr := slashArgs[0]
				cnt, err = strconv.Atoi(cntStr)
				if err != nil {
					log.Printf("'%v %v'", cntStr, slashArgs)
					tb.sendMessage(id, "로또 번호를 생성할 개수를 입력하세요.")
					continue
				}
			} else {
				cnt = 1
			}

			if cnt > 50 {
				tb.sendMessage(id, "한 번에 최대 50개까지 생성할 수 있습니다.")
				cnt = 50
			}

			tb.sendMessage(id, fmt.Sprintf("초지능의 힘으로 로또 번호 %d 개를 생성합니다...", cnt))
			lucky, err := tb.ai.PickNumFlow.Run(context.Background(), cnt)
			if err != nil {
				tb.sendMessage(id, fmt.Sprintf("로또 번호 생성 실패: %v", err))
				continue
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
			log.Printf("%s 의 요청으로 %d 개의 로또 번호를 초지능으로 생성했습니다.", username, cnt)
			tb.sendMessage(id, "생성완료")

		case "/rand", "/lotto":
			var cnt int
			if len(slashArgs) > 0 {
				var err error
				cntStr := slashArgs[0]
				cnt, err = strconv.Atoi(cntStr)
				if err != nil {
					log.Printf("'%v %v'", cntStr, slashArgs)
					tb.sendMessage(id, "로또 번호를 생성할 개수를 입력하세요.")
					continue
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
			log.Printf("%s 의 요청으로 %d 개의 로또 번호를 생성했습니다.", username, cnt)
			tb.sendMessage(id, "생성완료")

		case "/stat":
			tb.sendMessage(id, fmt.Sprintf("AI 생성 횟수: %d\n랜덤 생성 횟수: %d", tb.aiGenCnt, tb.randGenCnt))

		default:
			// tb.sendMessage(id, fmt.Sprintf("몰-루 : '%s'", update.Message.Text))
			tb.sendMessage(id, "Usage:\n/ai [count] - AI 로또 번호 생성\n/rand [count] - 랜덤 로또 번호 생성\n/stat - 통계 확인")
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
