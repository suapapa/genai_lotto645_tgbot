package main

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Prompt struct {
	Reterieve string `yaml:"retrieve"`
	System    string `yaml:"system"`
	User      string `yaml:"user_fmt"`
}

type Config struct {
	TelegramAPIToken string  `yaml:"tg_api_token"`
	ChatIDs          []int64 `yaml:"tg_chat_ids"`
	Prompt           *Prompt `yaml:"-"`
}

func loadConfig(fp, pp string) (*Config, error) {
	c := &Config{}
	f, err := os.Open(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(c); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}
	if c.TelegramAPIToken == "" {
		return nil, fmt.Errorf("missing Telegram API token")
	}
	if len(c.ChatIDs) == 0 {
		return nil, fmt.Errorf("missing Telegram chat IDs")
	}

	pt := &Prompt{}
	p, err := os.Open(pp)
	if err != nil {
		return nil, fmt.Errorf("failed to open prompt file: %w", err)
	}
	defer p.Close()

	if err := yaml.NewDecoder(p).Decode(pt); err != nil {
		return nil, fmt.Errorf("failed to decode prompt file: %w", err)
	}

	c.Prompt = pt

	return c, nil
}
