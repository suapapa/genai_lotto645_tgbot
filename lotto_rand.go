package main

import (
	"math/rand"
	"sort"
	"time"
)

var (
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// 1~45 중 cnt 개의 로또 번호를 생성합니다.
func generateLottoNumbers(cnt int) []int {
	if cnt < 1 {
		return nil
	}
	numbers := r.Perm(45)[:cnt]
	for i := range numbers {
		numbers[i]++
	}
	sort.Ints(numbers)
	return numbers
}
