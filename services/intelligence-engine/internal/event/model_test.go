package event

import "testing"

// Regression tests written BEFORE replacing incidentdetector's and
// correlationmodel's own classification logic with calls to this function,
// per M2.7.2's explicit sequencing requirement -- these lock in the exact
// semantics already proven correct in incidentdetector.ProcessEvent (fixed
// in M2.7.1) before that logic is extracted and reused elsewhere.
func TestIsErrorStatus(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		attributes map[string]string
		want       bool
	}{
		{"explicit ERROR status", "ERROR", nil, true},
		{"explicit 5xx status", "5xx", nil, true},
		{"UNSET status, no attributes", "UNSET", nil, false},
		{"UNSET status, 2xx attribute", "UNSET", map[string]string{"status": "201"}, false},
		{"UNSET status, 4xx attribute is not an error", "UNSET", map[string]string{"status": "409"}, false},
		{"UNSET status, micrometer 'status' attribute 5xx", "UNSET", map[string]string{"status": "500"}, true},
		{"UNSET status, http.response.status_code attribute 5xx", "UNSET", map[string]string{"http.response.status_code": "503"}, true},
		{"UNSET status, http.status_code attribute 5xx", "UNSET", map[string]string{"http.status_code": "502"}, true},
		{"attribute fallback applies regardless of status value, matching original semantics", "OK", map[string]string{"status": "500"}, true},
		{"empty status value in attribute is not an error", "UNSET", map[string]string{"status": ""}, false},
		{"unrelated attributes present, none matching", "UNSET", map[string]string{"method": "POST", "uri": "/api/payments"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsErrorStatus(c.status, c.attributes); got != c.want {
				t.Errorf("IsErrorStatus(%q, %v) = %v, want %v", c.status, c.attributes, got, c.want)
			}
		})
	}
}
