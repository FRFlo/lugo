package lsp

import "strings"

// fiveMSQLAsyncAlternative returns an asynchronous counterpart only when the
// adapter catalog identifies that exact synchronous call. Unknown adapters
// deliberately get no guessed migration target.
func fiveMSQLAsyncAlternative(name string) string {
	for _, adapter := range defaultFiveMSQLAdapters {
		for _, syncCall := range adapter.SyncCalls {
			if !strings.EqualFold(syncCall, name) {
				continue
			}
			lower := strings.ToLower(syncCall)
			for _, asyncCall := range adapter.Calls {
				if strings.EqualFold(asyncCall, syncCall) {
					continue
				}
				// Only claim a counterpart when its operation suffix matches.
				suffix := lower[strings.LastIndex(lower, ".")+1:]
				if strings.HasSuffix(strings.ToLower(asyncCall), "."+suffix) {
					return asyncCall
				}
			}
		}
	}
	return ""
}

func performanceSQLAdvice(name string) string {
	if alternative := fiveMSQLAsyncAlternative(name); alternative != "" {
		return "Synchronous SQL call may block the FiveM thread; migrate to the recognized asynchronous equivalent " + alternative + " and handle its callback/result."
	}
	return "Synchronous SQL-like call may block the FiveM thread; use an asynchronous API only after confirming the adapter documents an equivalent operation, then handle its callback/result."
}
