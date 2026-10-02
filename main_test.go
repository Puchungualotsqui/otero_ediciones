package main

import (
	"net/url"
	"testing"
)

func TestNormalizeSearch(t *testing.T) {
	got := normalize("Así_era-la vida")
	if got != "asi era la vida" {
		t.Fatalf("normalize() = %q, want %q", got, "asi era la vida")
	}
}

func TestSearchIgnoresAccentsAndSeparators(t *testing.T) {
	store := Store{
		Books: []Book{
			{SimplifiedName: "el_sindrome_de_heraclito", Titulo: "El síndrome de Heráclito", Autor: "Roger Otero Lorent"},
		},
	}
	result := store.search(url.Values{"busqueda": {"sindrome heraclito"}})
	if len(result.Books) != 1 || result.Books[0].SimplifiedName != "el_sindrome_de_heraclito" {
		t.Fatalf("search() returned %#v, want the matching book", result.Books)
	}
}

func TestEmbeddedSynopsisFiles(t *testing.T) {
	store := Store{}
	if !store.synopsisExists("xq") {
		t.Fatal("xq synopsis should be embedded")
	}
	if store.synopsisExists("cuarentena") {
		t.Fatal("cuarentena should remain unavailable until its synopsis is supplied")
	}
}

func TestSearchPaginatesInNineBookPages(t *testing.T) {
	books := make([]Book, 10)
	for index := range books {
		books[index].SimplifiedName = "book"
		books[index].Titulo = "Book"
	}
	result := (Store{Books: books}).search(url.Values{})
	if len(result.Books) != 9 || !result.HasMore || result.NextStartIndex != 9 {
		t.Fatalf("first page = len %d, hasMore %v, next %d", len(result.Books), result.HasMore, result.NextStartIndex)
	}
}
