// Package billing prices, meters, and charges every request that leaves the
// proxy. Nothing was doing this before: RecordUsage and LogRequest existed but
// had no callers, so usage_records and request_logs stayed empty, quota checks
// summed zero rows and always passed, and there was no data to invoice from.
package billing

// Nano is an amount of money in nanodollars — 1 USD = 1_000_000_000 Nano.
//
// Token pricing sits far below one cent per request: gpt-4o-mini input runs
// $0.15 per million tokens, so a 1,000-token prompt costs 0.015 cents. The old
// usage_records.cost_cents column stored an INTEGER number of cents, which
// truncates every realistic request to zero. int64 nanodollars leave eight
// digits of headroom below a cent and still count to $9.2 billion.
type Nano int64

const (
	// NanoPerUSD is the scale factor between dollars and the ledger unit.
	NanoPerUSD Nano = 1_000_000_000
	// NanoPerCent is one US cent.
	NanoPerCent Nano = NanoPerUSD / 100
	// tokensPerMTok is the denominator of the per-million-token price columns.
	tokensPerMTok int64 = 1_000_000
	// bpsScale is the denominator for basis-point margins (10_000 bps = 100%).
	bpsScale int64 = 10_000
)

// USD converts a dollar amount to Nano. Use for config and admin input only —
// arithmetic on money should stay in Nano.
func USD(v float64) Nano { return Nano(v * float64(NanoPerUSD)) }

// USD returns the amount as dollars, for display and JSON responses.
func (n Nano) USD() float64 { return float64(n) / float64(NanoPerUSD) }

// Cents returns the amount as (possibly fractional) cents.
func (n Nano) Cents() float64 { return float64(n) / float64(NanoPerCent) }
