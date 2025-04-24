package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Winning struct {
	IssueNo     int   `json:"issue_no" yaml:"issue_no"`
	Numbers     []int `json:"numbers" yaml:"numbers"`
	Bonus       int   `json:"bonus" yaml:"bonus"`
	FirstPrize  int   `json:"first_prize" yaml:"first_prize"`
	FirstCount  int   `json:"first_count" yaml:"first_count"`
	SecondPrize int   `json:"second_prize" yaml:"second_prize"`
	SecondCount int   `json:"second_count" yaml:"second_count"`
}

type WinningHistory []*Winning

func (wh WinningHistory) Len() int {
	return len(wh)
}
func (wh WinningHistory) Less(i, j int) bool {
	return wh[i].IssueNo < wh[j].IssueNo
}
func (wh WinningHistory) Swap(i, j int) {
	wh[i], wh[j] = wh[j], wh[i]
}

func loadWinningHistory(filePath string) (WinningHistory, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// Read the CSV file
	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	winningHistory := make(WinningHistory, 0, len(records)-1)
	// skip the header
	// 회차,번호1,번호2,번호3,번호4,번호5,번호6,보너스,1등 당첨금,1등 당첨수,2등 당첨금,2등 당첨수
	for _, record := range records[1:] {
		if len(record) < 12 {
			continue
		}
		var winning Winning
		winning.IssueNo, err = strconv.Atoi(record[0])
		if err != nil {
			return nil, fmt.Errorf("failed to parse issue number: %w", err)
		}
		for i := 1; i <= 6; i++ {
			num, err := strconv.Atoi(record[i])
			if err != nil {
				return nil, fmt.Errorf("failed to parse number: %w", err)
			}
			winning.Numbers = append(winning.Numbers, num)
		}
		bonus, err := strconv.Atoi(record[7])
		if err != nil {
			return nil, fmt.Errorf("failed to parse bonus: %w", err)
		}
		winning.Bonus = bonus
		firstPrizeStr := strings.ReplaceAll(record[8], ",", "")
		firstPrize, err := strconv.Atoi(firstPrizeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse first prize: %w", err)
		}
		secondPrizeStr := strings.ReplaceAll(record[10], ",", "")
		secondPrize, err := strconv.Atoi(secondPrizeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse second prize: %w", err)
		}
		firstCount, err := strconv.Atoi(record[9])
		if err != nil {
			return nil, fmt.Errorf("failed to parse first count: %w", err)
		}
		secondCount, err := strconv.Atoi(record[11])
		if err != nil {
			return nil, fmt.Errorf("failed to parse second count: %w", err)
		}

		winning.FirstPrize = firstPrize
		winning.FirstCount = firstCount
		winning.SecondPrize = secondPrize
		winning.SecondCount = secondCount

		winningHistory = append(winningHistory, &winning)
	}

	sort.Sort(winningHistory)

	return winningHistory, nil
}
