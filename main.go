package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/schollz/progressbar/v3"
)

var (
	flagBulkIndexCSV string
)

func main() {
	flag.StringVar(&flagBulkIndexCSV, "b", "", "CSV file to bulk index")
	flag.Parse()

	var cfg *Config
	var err error
	if cfg, err = loadConfig(".config.yaml"); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	lottoAI, err := NewLottoAI(
		cfg.Prompt.Reterieve,
		cfg.Prompt.System,
		cfg.Prompt.User,
	)
	if err != nil {
		log.Fatalf("failed to create LottoAI: %v", err)
	}

	if flagBulkIndexCSV != "" {
		// 그간의 당첨 정보를 익덱싱 합니다
		log.Println("Loading winning history...")
		winHistory, err := loadWinningHistory(flagBulkIndexCSV) // _data/lotto_history.csv
		if err != nil {
			log.Fatalf("failed to load winning history: %v", err)
		}
		log.Printf("Indexing %d winning history...\n", len(winHistory))
		bar := progressbar.Default(int64(len(winHistory)))
		ctx := context.Background()
		for _, w := range winHistory {
			bar.Add(1)
			lottoAI.IndexFlow.Run(ctx, w)
		}
	}

	// 텔레그렘 봇 시작
	log.Println("Starting Telegram bot...")
	tb, err := NewTelegramBot(
		lottoAI,
		cfg.TelegramAPIToken, cfg.ChatIDs...,
	)
	if err != nil {
		log.Fatalf("failed to create Telegram bot: %v", err)
	}
	defer tb.Close()

	// Start the bot
	go tb.Listen()

	log.Println("Telegram bot started. Listening for commands...")
	sysCh := make(chan os.Signal, 1)
	signal.Notify(sysCh, syscall.SIGINT, syscall.SIGTERM)
	<-sysCh
	log.Println("Received shutdown signal. Shutting down...")
}
