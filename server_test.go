package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLicenseEndpointServesProjectLicense(t *testing.T) {
	recorder := httptest.NewRecorder()
	newServer(&Catalog{}).handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/license", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "GNU GENERAL PUBLIC LICENSE") {
		t.Fatalf("license endpoint status=%d or response does not contain GPL text", recorder.Code)
	}
}

func TestNormalizeSearchTextCollapsesWhitespace(t *testing.T) {
	got := normalizeSearchText("  Anna   Marie\tSmith  ")
	if got != "anna marie smith" {
		t.Fatalf("normalizeSearchText() = %q, want %q", got, "anna marie smith")
	}
}

func TestSearchExactWhitespaceAndFuzzyFallback(t *testing.T) {
	county := &County{ID: 0, Name: "North County"}
	school := &School{ID: 0, Name: "Example School", County: county}
	class := &Class{
		ID: 0, Name: "Class One", School: school,
		People: []Person{{Name: "Anna  Marie Smith", Position: "Teacher"}},
	}
	county.Schools = []*School{school}
	school.Classes = []*Class{class}
	catalog := &Catalog{
		Counties: []*County{county}, Schools: []*School{school}, Classes: []*Class{class}, PeopleCount: 1,
	}
	handler := newServer(catalog).handler()
	search := func(query string) searchResponse {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/search?q="+url.QueryEscape(query), nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatalf("search status = %d, want 200", response.Code)
		}
		var result searchResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode search response: %v", err)
		}
		return result
	}

	for _, query := range []string{"Anna Marie Smith", "Anna  Marie Smith", "Anna\tMarie   Smith", "aNNa marie SMITH"} {
		result := search(query)
		if result.Similar || result.Total != 1 || result.Results[0].Kind != "person" {
			t.Fatalf("exact search for whitespace variant returned unexpected result: similar=%v total=%d", result.Similar, result.Total)
		}
	}

	for _, query := range []string{"Anna Marie Smiht", "Anna Marie Smoth", "Anna Marie Sith"} {
		fuzzy := search(query)
		if !fuzzy.Similar || fuzzy.Total != 1 || fuzzy.Results[0].Kind != "person" {
			t.Fatalf("fuzzy fallback for one-edit variant returned unexpected result: similar=%v total=%d", fuzzy.Similar, fuzzy.Total)
		}
	}

	tooManyEdits := search("Anna Merrie Smiht")
	if tooManyEdits.Similar || tooManyEdits.Total != 0 {
		t.Fatalf("query with multiple typos unexpectedly matched: similar=%v total=%d", tooManyEdits.Similar, tooManyEdits.Total)
	}

	short := search("Ann Smth")
	if short.Similar || short.Total != 0 {
		t.Fatalf("short ambiguous query unexpectedly used fuzzy matching: similar=%v total=%d", short.Similar, short.Total)
	}
}
