package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed web/* LICENSE
var webFiles embed.FS

type server struct {
	catalog     *Catalog
	searchIndex []searchEntry
	mux         *http.ServeMux
}

type summaryResponse struct {
	CountyCount  int `json:"countyCount"`
	SchoolCount  int `json:"schoolCount"`
	ClassCount   int `json:"classCount"`
	PeopleCount  int `json:"peopleCount"`
	PhotoMatches int `json:"photoMatches"`
	PhotoMissing int `json:"photoMissing"`
}

type treeResponse struct {
	Summary  summaryResponse `json:"summary"`
	Counties []countyView    `json:"counties"`
}

type countyView struct {
	ID      int          `json:"id"`
	Name    string       `json:"name"`
	Schools []schoolView `json:"schools"`
}

type schoolView struct {
	ID      int         `json:"id"`
	Name    string      `json:"name"`
	Classes []classView `json:"classes"`
}

type classView struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	PersonCount    int    `json:"personCount"`
	PhotoAvailable bool   `json:"photoAvailable"`
}

type classResponse struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	County         string   `json:"county"`
	School         string   `json:"school"`
	People         []Person `json:"people"`
	PhotoAvailable bool     `json:"photoAvailable"`
}

type searchResult struct {
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	CountyID int    `json:"countyId"`
	SchoolID int    `json:"schoolId"`
	ClassID  int    `json:"classId"`
}

type searchResponse struct {
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Similar bool           `json:"similar"`
	Results []searchResult `json:"results"`
}

type searchEntry struct {
	needle     string
	fuzzyWords []string
	result     searchResult
}

func newServer(catalog *Catalog) *server {
	s := &server{catalog: catalog, searchIndex: buildSearchIndex(catalog), mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /", s.handleHome)
	s.mux.HandleFunc("GET /license", s.handleLicense)
	s.mux.HandleFunc("GET /api/tree", s.handleTree)
	s.mux.HandleFunc("GET /api/class", s.handleClass)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /photo", s.handlePhoto)
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return s
}

func (s *server) handler() http.Handler {
	return s.mux
}

func (s *server) handleHome(w http.ResponseWriter, _ *http.Request) {
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "viewer page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *server) handleLicense(w http.ResponseWriter, _ *http.Request) {
	data, err := webFiles.ReadFile("LICENSE")
	if err != nil {
		http.Error(w, "license text unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *server) handleTree(w http.ResponseWriter, _ *http.Request) {
	response := treeResponse{
		Summary: summaryResponse{
			CountyCount: len(s.catalog.Counties), SchoolCount: len(s.catalog.Schools),
			ClassCount: len(s.catalog.Classes), PeopleCount: s.catalog.PeopleCount,
			PhotoMatches: s.catalog.PhotoMatches, PhotoMissing: s.catalog.PhotoMissing,
		},
		Counties: make([]countyView, 0, len(s.catalog.Counties)),
	}
	for _, county := range s.catalog.Counties {
		cv := countyView{ID: county.ID, Name: county.Name, Schools: make([]schoolView, 0, len(county.Schools))}
		for _, school := range county.Schools {
			sv := schoolView{ID: school.ID, Name: school.Name, Classes: make([]classView, 0, len(school.Classes))}
			for _, class := range school.Classes {
				sv.Classes = append(sv.Classes, classView{
					ID: class.ID, Name: class.Name, PersonCount: len(class.People), PhotoAvailable: class.PhotoAvailable,
				})
			}
			cv.Schools = append(cv.Schools, sv)
		}
		response.Counties = append(response.Counties, cv)
	}
	writeJSON(w, response)
}

func (s *server) handleClass(w http.ResponseWriter, r *http.Request) {
	class, ok := s.classFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, classResponse{
		ID: class.ID, Name: class.Name, County: class.School.County.Name,
		School: class.School.Name, People: class.People, PhotoAvailable: class.PhotoAvailable,
	})
}

func (s *server) handlePhoto(w http.ResponseWriter, r *http.Request) {
	class, ok := s.classFromRequest(w, r)
	if !ok {
		return
	}
	if !class.PhotoAvailable || class.PicturePath == "" {
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(class.PicturePath)
	if err != nil {
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "photo not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, filepath.Base(class.PicturePath), info.ModTime(), f)
}

func (s *server) classFromRequest(w http.ResponseWriter, r *http.Request) (*Class, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 0 || id >= len(s.catalog.Classes) {
		http.Error(w, "invalid class id", http.StatusBadRequest)
		return nil, false
	}
	return s.catalog.Classes[id], true
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	queryRunes := []rune(query)
	if len(queryRunes) > 200 {
		query = string(queryRunes[:200])
	}
	query = normalizeSearchText(query)
	response := searchResponse{Query: query, Results: []searchResult{}}
	if len([]rune(query)) < 2 {
		writeJSON(w, response)
		return
	}
	queryWords := strings.Fields(query)
	const resultLimit = 200
	for _, entry := range s.searchIndex {
		if strings.Contains(entry.needle, query) {
			response.Total++
			if len(response.Results) < resultLimit {
				response.Results = append(response.Results, entry.result)
			}
		}
	}
	if response.Total == 0 && fuzzyQueryEligible(queryWords) {
		type rankedResult struct {
			distance int
			result   searchResult
		}
		matches := make([]rankedResult, 0)
		for _, entry := range s.searchIndex {
			if distance := fuzzyNameDistance(queryWords, entry.fuzzyWords); distance >= 0 {
				matches = append(matches, rankedResult{distance: distance, result: entry.result})
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].distance != matches[j].distance {
				return matches[i].distance < matches[j].distance
			}
			return strings.ToLower(matches[i].result.Label) < strings.ToLower(matches[j].result.Label)
		})
		response.Total = len(matches)
		response.Similar = len(matches) > 0
		for i := 0; i < len(matches) && i < resultLimit; i++ {
			response.Results = append(response.Results, matches[i].result)
		}
	}
	writeJSON(w, response)
}

func buildSearchIndex(catalog *Catalog) []searchEntry {
	entries := make([]searchEntry, 0, len(catalog.Counties)+len(catalog.Schools)+len(catalog.Classes)+catalog.PeopleCount)
	add := func(searchableText, fuzzyText string, result searchResult) {
		entries = append(entries, searchEntry{
			needle:     normalizeSearchText(searchableText),
			fuzzyWords: strings.Fields(normalizeSearchText(fuzzyText)),
			result:     result,
		})
	}
	for _, county := range catalog.Counties {
		add(county.Name, county.Name, searchResult{
			Kind: "county", Label: county.Name, CountyID: county.ID, SchoolID: -1, ClassID: -1,
		})
	}
	for _, school := range catalog.Schools {
		add(school.Name, school.Name, searchResult{
			Kind: "school", Label: school.Name, Detail: school.County.Name,
			CountyID: school.County.ID, SchoolID: school.ID, ClassID: -1,
		})
	}
	for _, class := range catalog.Classes {
		add(class.Name, class.Name, searchResult{
			Kind: "class", Label: class.Name, Detail: class.School.County.Name + " / " + class.School.Name,
			CountyID: class.School.County.ID, SchoolID: class.School.ID, ClassID: class.ID,
		})
		for _, person := range class.People {
			add(person.Name+" "+person.Position, person.Name, searchResult{
				Kind: "person", Label: person.Name,
				Detail:   person.Position + " · " + class.School.County.Name + " / " + class.School.Name + " / " + class.Name,
				CountyID: class.School.County.ID, SchoolID: class.School.ID, ClassID: class.ID,
			})
		}
	}
	return entries
}

func normalizeSearchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func fuzzyQueryEligible(words []string) bool {
	if len(words) == 0 || len(words) > 5 {
		return false
	}
	for _, word := range words {
		if len([]rune(word)) < 4 {
			return false
		}
	}
	return true
}

// fuzzyNameDistance matches every query word to a distinct name word, allowing
// at most one edit in total. Adjacent transpositions count as one edit.
func fuzzyNameDistance(queryWords, nameWords []string) int {
	if len(queryWords) == 0 || len(queryWords) > len(nameWords) || len(nameWords) > 63 {
		return -1
	}
	return matchFuzzyWords(queryWords, nameWords, 0, 0, 0)
}

func matchFuzzyWords(queryWords, nameWords []string, queryIndex int, used uint64, edits int) int {
	if queryIndex == len(queryWords) {
		return edits
	}
	best := 2
	for i, nameWord := range nameWords {
		bit := uint64(1) << uint(i)
		if used&bit != 0 {
			continue
		}
		distance := fuzzyWordDistance(queryWords[queryIndex], nameWord)
		if distance < 0 || edits+distance > 1 {
			continue
		}
		result := matchFuzzyWords(queryWords, nameWords, queryIndex+1, used|bit, edits+distance)
		if result >= 0 && result < best {
			best = result
			if best == 0 {
				return 0
			}
		}
	}
	if best > 1 {
		return -1
	}
	return best
}

func fuzzyWordDistance(a, b string) int {
	left, right := []rune(a), []rune(b)
	if a == b {
		return 0
	}
	if len(left) < 4 || len(right) < 4 || abs(len(left)-len(right)) > 1 {
		return -1
	}
	if len(left) == len(right) {
		mismatches := make([]int, 0, 3)
		for i := range left {
			if left[i] != right[i] {
				mismatches = append(mismatches, i)
				if len(mismatches) > 2 {
					return -1
				}
			}
		}
		if len(mismatches) == 1 {
			return 1
		}
		if len(mismatches) == 2 {
			i, j := mismatches[0], mismatches[1]
			if j == i+1 && left[i] == right[j] && left[j] == right[i] {
				return 1
			}
		}
		return -1
	}
	shorter, longer := left, right
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	i, j, skipped := 0, 0, false
	for i < len(shorter) && j < len(longer) {
		if shorter[i] == longer[j] {
			i++
			j++
			continue
		}
		if skipped {
			return -1
		}
		skipped = true
		j++
	}
	return 1
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
