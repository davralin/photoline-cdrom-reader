package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func TestViewerAPIsServeCatalogAndPhoto(t *testing.T) {
	photoBytes := []byte("synthetic test photo")
	photoPath := filepath.Join(t.TempDir(), "class.jpg")
	if err := os.WriteFile(photoPath, photoBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	county := &County{ID: 0, Name: "Test County"}
	school := &School{ID: 0, Name: "Test School", County: county}
	class := &Class{
		ID: 0, Name: "Test Class", School: school,
		People: []Person{{Name: "Synthetic Person", Position: "Student"}},
		PhotoAvailable: true, PicturePath: photoPath,
	}
	county.Schools = []*School{school}
	school.Classes = []*Class{class}
	catalog := &Catalog{
		Counties: []*County{county}, Schools: []*School{school}, Classes: []*Class{class},
		PeopleCount: 1, PhotoMatches: 1,
	}
	handler := newServer(catalog).handler()

	t.Run("health", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("health status = %d, want %d", response.Code, http.StatusNoContent)
		}
	})

	t.Run("tree and summary", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tree", nil))
		var tree treeResponse
		if err := json.Unmarshal(response.Body.Bytes(), &tree); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || tree.Summary.CountyCount != 1 || tree.Summary.SchoolCount != 1 || tree.Summary.ClassCount != 1 || tree.Summary.PeopleCount != 1 {
			t.Fatalf("unexpected tree response status/counts: status=%d summary=%+v", response.Code, tree.Summary)
		}
		if len(tree.Counties) != 1 || len(tree.Counties[0].Schools) != 1 || len(tree.Counties[0].Schools[0].Classes) != 1 {
			t.Fatalf("catalog hierarchy was not returned: %+v", tree.Counties)
		}
	})

	t.Run("class detail", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/class?id=0", nil))
		var detail classResponse
		if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || detail.ID != 0 || detail.County != county.Name || detail.School != school.Name || len(detail.People) != 1 || !detail.PhotoAvailable {
			t.Fatalf("unexpected class detail status/response: status=%d id=%d people=%d photo=%v", response.Code, detail.ID, len(detail.People), detail.PhotoAvailable)
		}
	})

	t.Run("photo", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/photo?id=0", nil))
		if response.Code != http.StatusOK || response.Body.String() != string(photoBytes) {
			t.Fatalf("photo response status=%d or bytes differ", response.Code)
		}
	})

	t.Run("invalid class id", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/class?id=1", nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid class status = %d, want %d", response.Code, http.StatusBadRequest)
		}
	})
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
