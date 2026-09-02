package clipboard

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultPolicyIsValidAndWithinExactRanges(t *testing.T) {
	policy := DefaultPolicy()
	if err := ValidatePolicy(policy); err != nil {
		t.Fatalf("default policy rejected: %v", err)
	}
	if policy.AcquisitionDeadline != DefaultAcquisitionDeadline ||
		policy.OwnershipBudget != DefaultOwnershipBudget ||
		policy.KillGrace != DefaultKillGrace {
		t.Fatalf("default policy does not use declared defaults: %#v", policy)
	}
	if MinimumAcquisitionDeadline <= 0 || MinimumOwnershipBudget <= 0 || MinimumKillGrace <= 0 {
		t.Fatal("policy durations must be positive")
	}
	if DefaultAcquisitionDeadline < MinimumAcquisitionDeadline || DefaultAcquisitionDeadline > MaximumAcquisitionDeadline ||
		DefaultOwnershipBudget < MinimumOwnershipBudget || DefaultOwnershipBudget > MaximumOwnershipBudget ||
		DefaultKillGrace < MinimumKillGrace || DefaultKillGrace > MaximumKillGrace ||
		MinimumOwnershipBudget <= MaximumKillGrace {
		t.Fatal("declared ranges/defaults do not preserve the ownership budget invariant")
	}
}

func TestValidatePolicyAcceptsInclusiveBoundaries(t *testing.T) {
	tests := []Policy{
		{
			AcquisitionDeadline: MinimumAcquisitionDeadline,
			OwnershipBudget:     MinimumOwnershipBudget,
			KillGrace:           MinimumKillGrace,
		},
		{
			AcquisitionDeadline: MaximumAcquisitionDeadline,
			OwnershipBudget:     MaximumOwnershipBudget,
			KillGrace:           MaximumKillGrace,
		},
	}
	for _, policy := range tests {
		if err := ValidatePolicy(policy); err != nil {
			t.Fatalf("boundary policy %#v rejected: %v", policy, err)
		}
	}
}

func TestValidatePolicyRejectsInvalidValuesWithSafeSentinel(t *testing.T) {
	base := DefaultPolicy()
	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{name: "acquisition zero", mutate: func(p *Policy) { p.AcquisitionDeadline = 0 }},
		{name: "acquisition negative", mutate: func(p *Policy) { p.AcquisitionDeadline = -time.Second }},
		{name: "acquisition below minimum", mutate: func(p *Policy) { p.AcquisitionDeadline = MinimumAcquisitionDeadline - time.Nanosecond }},
		{name: "acquisition above maximum", mutate: func(p *Policy) { p.AcquisitionDeadline = MaximumAcquisitionDeadline + time.Nanosecond }},
		{name: "budget zero", mutate: func(p *Policy) { p.OwnershipBudget = 0 }},
		{name: "budget negative", mutate: func(p *Policy) { p.OwnershipBudget = -time.Second }},
		{name: "budget below minimum", mutate: func(p *Policy) { p.OwnershipBudget = MinimumOwnershipBudget - time.Nanosecond }},
		{name: "budget above maximum", mutate: func(p *Policy) { p.OwnershipBudget = MaximumOwnershipBudget + time.Nanosecond }},
		{name: "grace zero", mutate: func(p *Policy) { p.KillGrace = 0 }},
		{name: "grace negative", mutate: func(p *Policy) { p.KillGrace = -time.Second }},
		{name: "grace below minimum", mutate: func(p *Policy) { p.KillGrace = MinimumKillGrace - time.Nanosecond }},
		{name: "grace above maximum", mutate: func(p *Policy) { p.KillGrace = MaximumKillGrace + time.Nanosecond }},
		{name: "budget equals grace", mutate: func(p *Policy) { p.OwnershipBudget, p.KillGrace = 2*time.Second, 2*time.Second }},
		{name: "budget below grace", mutate: func(p *Policy) { p.OwnershipBudget, p.KillGrace = time.Second, 2*time.Second }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			test.mutate(&policy)
			if err := ValidatePolicy(policy); !errors.Is(err, ErrInvalidPolicy) || err != ErrInvalidPolicy {
				t.Fatalf("error = %v, want exact safe sentinel", err)
			}
		})
	}
}

func TestParseOwnershipBudgetSecondsBoundaries(t *testing.T) {
	tests := []struct {
		text string
		want time.Duration
	}{
		{text: "5", want: MinimumOwnershipBudget},
		{text: "00030", want: DefaultOwnershipBudget},
		{text: "120", want: MaximumOwnershipBudget},
	}
	for _, test := range tests {
		got, err := ParseOwnershipBudgetSeconds(test.text)
		if err != nil || got != test.want {
			t.Fatalf("ParseOwnershipBudgetSeconds(%q) = %v, %v; want %v, nil", test.text, got, err, test.want)
		}
	}
}

func TestParseOwnershipBudgetSecondsRejectsInvalidAndOverflow(t *testing.T) {
	for _, text := range []string{"", "0", "-1", "+5", "4", "121", "18446744073709551616", "999999999999999999999999999999999", " 5", "5 ", "5s", "5.0", "５"} {
		t.Run(text, func(t *testing.T) {
			got, err := ParseOwnershipBudgetSeconds(text)
			if got != 0 || err != ErrInvalidPolicy {
				t.Fatalf("result = %v, %v; want zero and exact safe sentinel", got, err)
			}
		})
	}
}
