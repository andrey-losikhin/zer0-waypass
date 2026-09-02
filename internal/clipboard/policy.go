package clipboard

import (
	"errors"
	"strconv"
	"time"
)

const (
	MinimumAcquisitionDeadline = 5 * time.Second
	DefaultAcquisitionDeadline = 30 * time.Second
	MaximumAcquisitionDeadline = 2 * time.Minute

	MinimumOwnershipBudget = 5 * time.Second
	DefaultOwnershipBudget = 30 * time.Second
	MaximumOwnershipBudget = 2 * time.Minute

	MinimumKillGrace = 150 * time.Millisecond
	DefaultKillGrace = 500 * time.Millisecond
	MaximumKillGrace = 2 * time.Second
)

// ErrInvalidPolicy intentionally carries no rejected value or configuration.
var ErrInvalidPolicy = errors.New("invalid clipboard policy")

// Policy defines the bounded acquisition and clipboard-ownership phases. It
// does not itself apply timers; the guardian lifecycle implements them.
type Policy struct {
	AcquisitionDeadline time.Duration
	OwnershipBudget     time.Duration
	KillGrace           time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		AcquisitionDeadline: DefaultAcquisitionDeadline,
		OwnershipBudget:     DefaultOwnershipBudget,
		KillGrace:           DefaultKillGrace,
	}
}

func ValidatePolicy(policy Policy) error {
	if policy.OwnershipBudget <= policy.KillGrace ||
		policy.AcquisitionDeadline < MinimumAcquisitionDeadline ||
		policy.AcquisitionDeadline > MaximumAcquisitionDeadline ||
		policy.OwnershipBudget < MinimumOwnershipBudget ||
		policy.OwnershipBudget > MaximumOwnershipBudget ||
		policy.KillGrace < MinimumKillGrace ||
		policy.KillGrace > MaximumKillGrace {
		return ErrInvalidPolicy
	}
	return nil
}

// ParseOwnershipBudgetSeconds parses the public --ttl value and enforces the
// same ownership-budget bounds used by Policy. Leading zeroes are accepted;
// signs, whitespace, suffixes, non-ASCII digits and out-of-range values are
// rejected.
func ParseOwnershipBudgetSeconds(text string) (time.Duration, error) {
	if text == "" {
		return 0, ErrInvalidPolicy
	}
	for index := range len(text) {
		if text[index] < '0' || text[index] > '9' {
			return 0, ErrInvalidPolicy
		}
	}

	seconds, err := strconv.ParseUint(text, 10, 64)
	if err != nil || seconds < uint64(MinimumOwnershipBudget/time.Second) ||
		seconds > uint64(MaximumOwnershipBudget/time.Second) {
		return 0, ErrInvalidPolicy
	}
	return time.Duration(seconds) * time.Second, nil
}
