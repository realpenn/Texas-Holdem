package poker

import (
	"fmt"
	"sort"
)

type HandRank struct {
	Value    uint64 `json:"value"`
	Category string `json:"category"`
	Cards    []Card `json:"cards"`
}

var categoryNames = []string{
	"高牌",
	"一对",
	"两对",
	"三条",
	"顺子",
	"同花",
	"葫芦",
	"四条",
	"同花顺",
}

func Evaluate(cards []Card) (HandRank, error) {
	if len(cards) < 5 || len(cards) > 7 {
		return HandRank{}, fmt.Errorf("evaluate needs 5 to 7 cards, got %d", len(cards))
	}
	best := HandRank{}
	for a := 0; a < len(cards)-4; a++ {
		for b := a + 1; b < len(cards)-3; b++ {
			for c := b + 1; c < len(cards)-2; c++ {
				for d := c + 1; d < len(cards)-1; d++ {
					for e := d + 1; e < len(cards); e++ {
						hand := []Card{cards[a], cards[b], cards[c], cards[d], cards[e]}
						rank := evaluateFive(hand)
						if rank.Value > best.Value {
							best = rank
						}
					}
				}
			}
		}
	}
	return best, nil
}

func CompareCards(a, b []Card) (int, error) {
	ra, err := Evaluate(a)
	if err != nil {
		return 0, err
	}
	rb, err := Evaluate(b)
	if err != nil {
		return 0, err
	}
	if ra.Value > rb.Value {
		return 1, nil
	}
	if ra.Value < rb.Value {
		return -1, nil
	}
	return 0, nil
}

func evaluateFive(cards []Card) HandRank {
	sortedCards := append([]Card(nil), cards...)
	sort.Slice(sortedCards, func(i, j int) bool {
		if sortedCards[i].Rank == sortedCards[j].Rank {
			return sortedCards[i].Suit > sortedCards[j].Suit
		}
		return sortedCards[i].Rank > sortedCards[j].Rank
	})
	ranks := make([]int, 0, 5)
	counts := map[int]int{}
	suits := map[int]int{}
	for _, c := range sortedCards {
		ranks = append(ranks, c.Rank)
		counts[c.Rank]++
		suits[c.Suit]++
	}
	flush := len(suits) == 1
	straightHigh := straightHigh(ranks)
	if flush && straightHigh > 0 {
		return makeRank(8, []int{straightHigh}, sortedCards)
	}

	groups := rankGroups(counts)
	if groups[0].Count == 4 {
		kicker := highestExcluding(ranks, groups[0].Rank)
		return makeRank(7, []int{groups[0].Rank, kicker}, sortedCards)
	}
	if groups[0].Count == 3 && groups[1].Count == 2 {
		return makeRank(6, []int{groups[0].Rank, groups[1].Rank}, sortedCards)
	}
	if flush {
		return makeRank(5, ranks, sortedCards)
	}
	if straightHigh > 0 {
		return makeRank(4, []int{straightHigh}, sortedCards)
	}
	if groups[0].Count == 3 {
		kickers := kickersExcluding(ranks, groups[0].Rank)
		return makeRank(3, append([]int{groups[0].Rank}, kickers...), sortedCards)
	}
	if groups[0].Count == 2 && groups[1].Count == 2 {
		highPair := groups[0].Rank
		lowPair := groups[1].Rank
		if lowPair > highPair {
			highPair, lowPair = lowPair, highPair
		}
		kicker := highestExcluding(ranks, highPair, lowPair)
		return makeRank(2, []int{highPair, lowPair, kicker}, sortedCards)
	}
	if groups[0].Count == 2 {
		kickers := kickersExcluding(ranks, groups[0].Rank)
		return makeRank(1, append([]int{groups[0].Rank}, kickers...), sortedCards)
	}
	return makeRank(0, ranks, sortedCards)
}

type rankGroup struct {
	Rank  int
	Count int
}

func rankGroups(counts map[int]int) []rankGroup {
	groups := make([]rankGroup, 0, len(counts))
	for rank, count := range counts {
		groups = append(groups, rankGroup{Rank: rank, Count: count})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].Rank > groups[j].Rank
		}
		return groups[i].Count > groups[j].Count
	})
	return groups
}

func straightHigh(ranks []int) int {
	seen := map[int]bool{}
	for _, r := range ranks {
		seen[r] = true
	}
	unique := make([]int, 0, len(seen)+1)
	for r := range seen {
		unique = append(unique, r)
	}
	if seen[14] {
		unique = append(unique, 1)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(unique)))
	run := 1
	for i := 1; i < len(unique); i++ {
		if unique[i] == unique[i-1]-1 {
			run++
			if run >= 5 {
				return unique[i-4]
			}
			continue
		}
		run = 1
	}
	return 0
}

func makeRank(category int, ranks []int, cards []Card) HandRank {
	value := uint64(category)
	for i := 0; i < 5; i++ {
		value *= 15
		if i < len(ranks) {
			value += uint64(ranks[i])
		}
	}
	return HandRank{
		Value:    value,
		Category: categoryNames[category],
		Cards:    append([]Card(nil), cards...),
	}
}

func highestExcluding(ranks []int, excluded ...int) int {
	kickers := kickersExcluding(ranks, excluded...)
	if len(kickers) == 0 {
		return 0
	}
	return kickers[0]
}

func kickersExcluding(ranks []int, excluded ...int) []int {
	ex := map[int]bool{}
	for _, r := range excluded {
		ex[r] = true
	}
	out := make([]int, 0, len(ranks))
	seen := map[int]bool{}
	for _, r := range ranks {
		if ex[r] || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
