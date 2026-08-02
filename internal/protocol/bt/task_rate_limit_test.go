package bt

import (
	"testing"
)

func TestApplyBTRateLimiters_ClearsPauseWhenUnlimited(t *testing.T) {
	t.Parallel()

	st := &state{
		rateLimitPausedDL: true,
		rateLimitPausedUL: true,
		options:           map[string]string{"max-download-limit": "8192"},
	}
	applyBTRateLimiters(st, map[string]string{"max-download-limit": "0", "max-upload-limit": "0"})
	if st.rateLimitPausedDL || st.rateLimitPausedUL {
		t.Fatal("expected pause flags cleared when limiters removed")
	}
	if st.downloadLimiter != nil || st.uploadLimiter != nil {
		t.Fatal("expected nil limiters for zero limits")
	}
}
