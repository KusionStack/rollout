package utils

import (
	"fmt"
	"math/rand"
	"time"
)

// RandStr returns a random string
func RandStr() string {
	randNum := rand.Intn(999)
	if randNum < 100 {
		randNum += 100
	}
	return fmt.Sprintf("%s%d", time.Now().Format("20060102150405"), randNum)
}
