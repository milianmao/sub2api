package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartitionAccountPoolPrefersStablePrimaryCandidates(t *testing.T) {
	primaryOne := &Account{ID: 1}
	fallback := &Account{ID: 2, IsFallback: true}
	primaryTwo := &Account{ID: 3}

	selected, role := PartitionAccountPool([]*Account{fallback, nil, primaryOne, primaryTwo}, func(account *Account) *Account {
		return account
	}).Preferred()

	require.Equal(t, AccountPoolPrimary, role)
	require.Equal(t, []*Account{primaryOne, primaryTwo}, selected)
}

func TestAccountMatchesPoolRejectsFallbackDirectHitAfterPrimaryRecovery(t *testing.T) {
	primary := &Account{ID: 10}
	fallback := &Account{ID: 11, IsFallback: true}

	require.True(t, AccountMatchesPool(primary, AccountPoolPrimary))
	require.False(t, AccountMatchesPool(fallback, AccountPoolPrimary))
	require.True(t, AccountMatchesPool(fallback, AccountPoolFallback))
	require.False(t, AccountMatchesPool(nil, AccountPoolFallback))
}

func TestPartitionAccountPoolUsesFallbackWhenNoPrimaryIsEligible(t *testing.T) {
	fallbackOne := &Account{ID: 1, IsFallback: true}
	fallbackTwo := &Account{ID: 2, IsFallback: true}

	selected, role := PartitionAccountPool([]*Account{nil, fallbackOne, fallbackTwo}, func(account *Account) *Account {
		return account
	}).Preferred()

	require.Equal(t, AccountPoolFallback, role)
	require.Equal(t, []*Account{fallbackOne, fallbackTwo}, selected)
}
