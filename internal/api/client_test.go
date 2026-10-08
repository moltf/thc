package api

import "testing"

func TestRetryable(t *testing.T) {
	codes := []int{
		429,
		500,
		502,
		503,
		504,
	}

	for _, code := range codes {
		if !retryable(code) {
			t.Errorf(
				"%d should be retryable",
				code,
			)
		}
	}

	if retryable(400) {
		t.Fatal("400 should not retry")
	}

	if retryable(404) {
		t.Fatal("404 should not retry")
	}
}
