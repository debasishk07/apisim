package inject

import (
	"fmt"
	"math/rand/v2"

	"apisim/internal/config"
)

// BuildInjectors converts config failure specs into Injector instances,
// sharing one RNG so probabilistic behavior is deterministic given a seed.
func BuildInjectors(fails []config.Failure, seed int64) ([]Injector, error) {
	rng := newSharedRNG(seed)
	var injects []Injector
	for _, f := range fails {
		inj, err := buildOne(f, rng)
		if err != nil {
			return nil, fmt.Errorf("failure %q: %w", f.Type, err)
		}
		injects = append(injects, inj)
	}
	return injects, nil
}

func newSharedRNG(seed int64) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
}

func buildOne(f config.Failure, rng *rand.Rand) (Injector, error) {
	switch f.Type {
	case "latency":
		return NewLatency(f.Latency.DelayMs, f.Latency.JitterMs, f.Trigger, rng)
	case "error":
		return NewError(f.Error.Status, f.Error.Body, f.Error.Headers, f.Trigger, rng)
	case "timeout":
		return NewTimeout(f.Trigger, rng)
	case "drop_connection":
		return NewDropConn(f.Trigger, rng)
	case "mutate_body":
		ops := []MutateOp{}
		for _, o := range f.MutateBody.Operations {
			ops = append(ops, MutateOp{Op: o.Op, Path: o.Path, To: o.To})
		}
		return NewMutateBody(ops, f.Trigger, rng)
	case "rate_limit":
		return NewRateLimit(f.RateLimit.Limit, f.RateLimit.WindowS, f.RateLimit.Status, f.RateLimit.Headers, f.Trigger, rng)
	case "auth_expiry":
		return NewAuthExpiry(f.AuthExpiry.AfterRequests, f.AuthExpiry.Status, f.Trigger, rng)
	case "slow_body":
		return NewSlowBody(f.SlowBody.BytesPerSecond, f.Trigger, rng)
	default:
		return nil, fmt.Errorf("unknown injector type %q", f.Type)
	}
}

var _ = newSharedRNG
