package service

// AccountPoolRole is an internal routing decision. It deliberately does not
// appear in administrator DTOs, which expose the durable IsFallback switch.
type AccountPoolRole string

const (
	AccountPoolPrimary  AccountPoolRole = "primary"
	AccountPoolFallback AccountPoolRole = "fallback"
)

// AccountPoolPartition preserves candidate ordering while separating the
// request-eligible candidates by global account role. Callers must apply every
// request-specific gate before this helper; concurrency is intentionally not a
// gate because primary wait plans must not spill into fallback capacity.
type AccountPoolPartition[T any] struct {
	Primary  []T
	Fallback []T
}

func PartitionAccountPool[T any](items []T, accountOf func(T) *Account) AccountPoolPartition[T] {
	out := AccountPoolPartition[T]{
		Primary:  make([]T, 0, len(items)),
		Fallback: make([]T, 0, len(items)),
	}
	for _, item := range items {
		account := accountOf(item)
		if account == nil {
			continue
		}
		if account.IsFallback {
			out.Fallback = append(out.Fallback, item)
		} else {
			out.Primary = append(out.Primary, item)
		}
	}
	return out
}

func (p AccountPoolPartition[T]) Preferred() ([]T, AccountPoolRole) {
	if len(p.Primary) > 0 {
		return p.Primary, AccountPoolPrimary
	}
	return p.Fallback, AccountPoolFallback
}

// PreferAccountPool applies the strict primary boundary to a candidate list.
// It is useful only after callers have completed their request eligibility
// checks; it must never be used to turn primary concurrency saturation into
// fallback eligibility.
func PreferAccountPool(accounts []Account) ([]Account, AccountPoolRole) {
	return PartitionAccountPool(accounts, func(account Account) *Account { return &account }).Preferred()
}

func AccountMatchesPool(account *Account, role AccountPoolRole) bool {
	if account == nil {
		return false
	}
	return (role == AccountPoolFallback) == account.IsFallback
}
