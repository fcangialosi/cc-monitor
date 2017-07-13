package main

import (
	"math/rand"
	"time"
)

type ExponentialDistribution struct {
	mean float64
	prng *rand.Rand
}

func getNewPRNG() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func createExpDist(mean float64, prng *rand.Rand) *ExponentialDistribution {
	return &ExponentialDistribution{
		mean: mean,
		prng: prng,
	}
}

func (e *ExponentialDistribution) Sample() float64 {
	return e.prng.ExpFloat64() / e.mean
}
