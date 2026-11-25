// PhotoLine CD-ROM Reader parses the legacy catalog for local, read-only browsing.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	dataStart       = 0x147
	stringType      = 0x103
	maxStringLength = 1 << 20
)

type Person struct {
	Name     string `json:"name"`
	Position string `json:"position"`
}

type County struct {
	ID      int       `json:"id"`
	Name    string    `json:"name"`
	Schools []*School `json:"schools"`
}

type School struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Prefix  string   `json:"-"`
	County  *County  `json:"-"`
	Classes []*Class `json:"classes"`
}

type Class struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	School           *School  `json:"-"`
	People           []Person `json:"people"`
	PictureReference string   `json:"-"`
	PicturePath      string   `json:"-"`
	PhotoAvailable   bool     `json:"photoAvailable"`
}

type Catalog struct {
	Counties      []*County
	Schools       []*School
	Classes       []*Class
	PeopleCount   int
	PhotoMatches  int
	PhotoMissing  int
	Unreferenced  int
	dataDirectory string
}

type stringRecord struct {
	offset int
	field  uint32
	value  string
}

// loadCatalog reads only the copied TextData.LST and the image directory.
func loadCatalog(dataDirectory string) (*Catalog, error) {
	dataDirectory, err := filepath.Abs(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dataDirectory, "TextData.LST"))
	if err != nil {
		return nil, fmt.Errorf("read TextData.LST: %w", err)
	}
	picturesDirectory := filepath.Join(dataDirectory, "Pictures")
	entries, err := os.ReadDir(picturesDirectory)
	if err != nil {
		return nil, fmt.Errorf("read Pictures directory: %w", err)
	}
	images := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jpg") {
			continue
		}
		key := strings.ToLower(entry.Name())
		if _, exists := images[key]; exists {
			return nil, fmt.Errorf("duplicate case-insensitive image filename in Pictures")
		}
		images[key] = filepath.Join(picturesDirectory, entry.Name())
	}

	return parseCatalog(data, dataDirectory, images)
}

func parseCatalog(data []byte, dataDirectory string, images map[string]string) (*Catalog, error) {
	records, err := extractStringRecords(data)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{dataDirectory: dataDirectory}
	var currentCounty *County
	var currentSchool *School
	var currentClass *Class
	var pendingPerson string
	counts := make(map[uint32]int)

	for _, record := range records {
		counts[record.field]++
		switch record.field {
		case 1: // sNameOfCounty
			currentCounty = &County{ID: len(catalog.Counties), Name: record.value}
			catalog.Counties = append(catalog.Counties, currentCounty)
			currentSchool, currentClass = nil, nil
		case 3: // sNameOfSchool
			if currentCounty == nil {
				return nil, fmt.Errorf("school record precedes its county at TextData.LST offset 0x%x", record.offset)
			}
			currentSchool = &School{ID: len(catalog.Schools), Name: record.value, County: currentCounty}
			currentCounty.Schools = append(currentCounty.Schools, currentSchool)
			catalog.Schools = append(catalog.Schools, currentSchool)
			currentClass = nil
		case 5: // sNameOfClass
			if currentSchool == nil {
				return nil, fmt.Errorf("class record precedes its school at TextData.LST offset 0x%x", record.offset)
			}
			if pendingPerson != "" {
				return nil, fmt.Errorf("incomplete person record before class at TextData.LST offset 0x%x", record.offset)
			}
			currentClass = &Class{ID: len(catalog.Classes), Name: record.value, School: currentSchool}
			currentSchool.Classes = append(currentSchool.Classes, currentClass)
			catalog.Classes = append(catalog.Classes, currentClass)
		case 7: // sName
			if currentClass == nil || pendingPerson != "" {
				return nil, fmt.Errorf("unexpected person-name field at TextData.LST offset 0x%x", record.offset)
			}
			pendingPerson = record.value
		case 8: // sPositon (source spelling)
			if currentClass == nil || pendingPerson == "" {
				return nil, fmt.Errorf("unexpected person-position field at TextData.LST offset 0x%x", record.offset)
			}
			currentClass.People = append(currentClass.People, Person{Name: pendingPerson, Position: record.value})
			pendingPerson = ""
			catalog.PeopleCount++
		case 10: // sClassPicture
			if currentClass == nil || currentClass.PictureReference != "" {
				return nil, fmt.Errorf("unexpected class-picture field at TextData.LST offset 0x%x", record.offset)
			}
			currentClass.PictureReference = record.value
			base := filepath.Base(strings.ReplaceAll(record.value, "\\", "/"))
			if path, ok := images[strings.ToLower(base)]; ok {
				currentClass.PicturePath = path
				currentClass.PhotoAvailable = true
				catalog.PhotoMatches++
			} else {
				catalog.PhotoMissing++
			}
		case 11: // sSchoolPreFix
			if currentSchool == nil || currentSchool.Prefix != "" {
				return nil, fmt.Errorf("unexpected school-prefix field at TextData.LST offset 0x%x", record.offset)
			}
			currentSchool.Prefix = record.value
		}
	}
	if pendingPerson != "" {
		return nil, errors.New("TextData.LST ends with an incomplete person record")
	}
	if len(catalog.Counties) == 0 || len(catalog.Schools) == 0 || len(catalog.Classes) == 0 {
		return nil, errors.New("TextData.LST contains no complete county/school/class hierarchy")
	}
	for _, school := range catalog.Schools {
		if school.Prefix == "" {
			return nil, fmt.Errorf("school record at index %d has no image prefix", school.ID)
		}
		for _, class := range school.Classes {
			if !strings.HasPrefix(strings.ToLower(filepath.Base(strings.ReplaceAll(class.PictureReference, "\\", "/"))), strings.ToLower(school.Prefix)) {
				return nil, fmt.Errorf("class image prefix does not match its school at class index %d", class.ID)
			}
		}
	}
	for _, class := range catalog.Classes {
		if class.PictureReference == "" {
			return nil, fmt.Errorf("class record at index %d has no picture reference", class.ID)
		}
	}
	catalog.Unreferenced = len(images) - catalog.PhotoMatches
	if catalog.Unreferenced < 0 {
		catalog.Unreferenced = 0
	}
	if counts[7] != counts[8] || counts[5] != counts[10] {
		return nil, fmt.Errorf("TextData.LST field totals are inconsistent (classes=%d/%d, people=%d/%d)", counts[5], counts[10], counts[7], counts[8])
	}
	return catalog, nil
}

func extractStringRecords(data []byte) ([]stringRecord, error) {
	if len(data) <= dataStart {
		return nil, errors.New("TextData.LST is shorter than its catalog header")
	}
	records := make([]stringRecord, 0, 100000)
	for offset := dataStart; offset+12 <= len(data); offset++ {
		field := binary.BigEndian.Uint32(data[offset : offset+4])
		if field != 1 && field != 3 && field != 5 && field != 7 && field != 8 && field != 10 && field != 11 {
			continue
		}
		if binary.BigEndian.Uint32(data[offset+4:offset+8]) != stringType {
			continue
		}
		length := binary.BigEndian.Uint32(data[offset+8 : offset+12])
		if length == 0 || length > maxStringLength || uint64(offset)+12+uint64(length) > uint64(len(data)) {
			continue
		}
		value := data[offset+12 : offset+12+int(length)]
		if value[len(value)-1] != 0 || strings.IndexByte(string(value[:len(value)-1]), 0) >= 0 {
			continue
		}
		decoded := strings.TrimSpace(decodeLegacyText(value[:len(value)-1]))
		if decoded == "" {
			continue
		}
		records = append(records, stringRecord{offset: offset, field: field, value: decoded})
	}
	if len(records) == 0 {
		return nil, errors.New("no supported string records found in TextData.LST")
	}
	return records, nil
}

func decodeLegacyText(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	const cp1252 = "\u20ac\u0081\u201a\u0192\u201e\u2026\u2020\u2021\u02c6\u2030\u0160\u2039\u0152\u008d\u017d\u008f\u0090\u2018\u2019\u201c\u201d\u2022\u2013\u2014\u02dc\u2122\u0161\u203a\u0153\u009d\u017e\u0178"
	var out strings.Builder
	out.Grow(len(data))
	for _, value := range data {
		if value >= 0x80 && value <= 0x9f {
			out.WriteRune([]rune(cp1252)[value-0x80])
		} else {
			out.WriteRune(rune(value))
		}
	}
	return out.String()
}
