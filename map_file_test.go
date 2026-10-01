package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Saving a generated level from the editor unlinks it from its build.map, so
// the save must keep everything the generator wrote: a field mapgen puts in
// map.json that the editor's own format does not carry is gone after the first
// save, with nothing left to build it again from.
func TestAnEditorSaveKeepsAGeneratedLevel(t *testing.T) {
	path := filepath.Join(mapDir, "ark", "map.json")
	generated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadJSONMap(path)
	if err != nil {
		t.Fatal(err)
	}
	resaved := filepath.Join(t.TempDir(), "map.json")
	if err := saveJSONMap(resaved, loaded); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(resaved)
	if err != nil {
		t.Fatal(err)
	}

	var before, after map[string]any
	if err := json.Unmarshal(generated, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(again, &after); err != nil {
		t.Fatal(err)
	}
	delete(before, "Edited")
	delete(after, "Edited")
	for key, value := range before {
		if !reflect.DeepEqual(value, after[key]) {
			t.Errorf("%s did not survive an editor save", key)
		}
	}
	for key := range after {
		if _, generatedToo := before[key]; !generatedToo {
			t.Errorf("an editor save wrote %s, which the generator does not", key)
		}
	}
}
