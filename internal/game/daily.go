package game

import (
	"fmt"
	"hash/fnv"
	"time"
)

// DailySeed produces the same seed for all players on the same UTC day,
// city, game, and mode. The dataset still needs to be identical for a
// truly comparable daily leaderboard.
func DailySeed(day time.Time, city, name, mode string) int64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "funda-daily-v1|%s|%s|%s|%s", day.UTC().Format("2006-01-02"), city, name, mode)
	return int64(h.Sum64() & 0x7fffffffffffffff)
}
