package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeLegacyText(t *testing.T) {
	got := decodeLegacyText([]byte{0x4e, 0xf8, 0x72})
	if got != "Nør" {
		t.Fatalf("decodeLegacyText() = %q, want %q", got, "Nør")
	}
	if got := decodeLegacyText([]byte("Ålesund")); got != "Ålesund" {
		t.Fatalf("valid UTF-8 changed: %q", got)
	}
}

func TestParseCatalogBuildsHierarchyAndMatchesPhoto(t *testing.T) {
	data := make([]byte, dataStart)
	appendString := func(field uint32, value string) {
		var header [12]byte
		binary.BigEndian.PutUint32(header[0:4], field)
		binary.BigEndian.PutUint32(header[4:8], stringType)
		binary.BigEndian.PutUint32(header[8:12], uint32(len(value)+1))
		data = append(data, header[:]...)
		data = append(data, value...)
		data = append(data, 0)
	}
	appendString(1, "Test County")
	appendString(3, "Example School")
	appendString(5, "Class A")
	appendString(7, "Student")
	appendString(8, "Member")
	appendString(10, `Pictures\0001_Class_A.jpg`)
	appendString(11, "0001_")

	images := map[string]string{"0001_class_a.jpg": filepath.Join(t.TempDir(), "0001_Class_A.jpg")}
	cat, err := parseCatalog(data, t.TempDir(), images)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Counties) != 1 || len(cat.Schools) != 1 || len(cat.Classes) != 1 || cat.PeopleCount != 1 {
		t.Fatalf("unexpected catalog counts: counties=%d schools=%d classes=%d people=%d", len(cat.Counties), len(cat.Schools), len(cat.Classes), cat.PeopleCount)
	}
	class := cat.Classes[0]
	if class.School.County.Name != "Test County" || class.School.Name != "Example School" || class.Name != "Class A" {
		t.Fatalf("hierarchy was not retained: %#v", class)
	}
	if len(class.People) != 1 || class.People[0] != (Person{Name: "Student", Position: "Member"}) {
		t.Fatalf("person pair was not retained: %#v", class.People)
	}
	if !class.PhotoAvailable || cat.PhotoMatches != 1 {
		t.Fatalf("photo reference did not resolve: %#v", class)
	}
}

func TestLoadCatalogUsesOnlyJPGImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Pictures"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, dataStart)
	for field, value := range []string{"County", "School", "Class", "Name", "Position", `Pictures\0001_Class.jpg`, "0001_"} {
		fields := []uint32{1, 3, 5, 7, 8, 10, 11}
		var header [12]byte
		binary.BigEndian.PutUint32(header[0:4], fields[field])
		binary.BigEndian.PutUint32(header[4:8], stringType)
		binary.BigEndian.PutUint32(header[8:12], uint32(len(value)+1))
		data = append(data, header[:]...)
		data = append(data, value...)
		data = append(data, 0)
	}
	if err := os.WriteFile(filepath.Join(dir, "TextData.LST"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Pictures", "0001_Class.jpg"), []byte("not a real image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Pictures", "Thumbs.db"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := loadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cat.PhotoMatches != 1 || cat.PhotoMissing != 0 || cat.Unreferenced != 0 {
		t.Fatalf("unexpected photo counts: matches=%d missing=%d unreferenced=%d", cat.PhotoMatches, cat.PhotoMissing, cat.Unreferenced)
	}
}
