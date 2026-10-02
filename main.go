package main

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// The unified app ships with its content and frontend assets. It can therefore
// run as a single binary without a separate frontend build or container.
//
//go:embed books.tsv frontpage_categories.json templates static content/synopsis
var siteFiles embed.FS

const pageSize = 9

type Book struct {
	SimplifiedName string `json:"simplified_name"`
	Titulo         string `json:"titulo"`
	Autor          string `json:"autor"`
	NivelEducativo string `json:"nivel_educativo"`
	Materia        string `json:"materia"`
	Tipo           string `json:"tipo"`
	Idioma         string `json:"idioma"`
	Ilustraciones  string `json:"ilustraciones"`
	Genero         string `json:"genero"`
	Paginas        string `json:"paginas"`
	Tamano         string `json:"tamano"`
	DepositoLegal  string `json:"deposito_legal"`
	ISBN           string `json:"isbn"`
	Edad           string `json:"edad"`
	FichaDidactica string `json:"ficha_didactica"`
}

type Category struct {
	Titulo string   `json:"titulo"`
	Libros []string `json:"libros"`
}

type HomeRow struct {
	Titulo string
	Books  []Book
}

type CatalogResult struct {
	Books          []Book
	HasMore        bool
	StartIndex     int
	NextStartIndex int
}

type Page struct {
	Title       string
	Description string
	Path        string
	Query       url.Values
	HomeRows    []HomeRow
	Catalog     CatalogResult
	Book        *Book
	SynopsisURL string
}

type Store struct {
	Books      []Book
	Categories []Category
	BySlug     map[string]Book
	BaseURL    string
}

var translations = map[string]string{
	"NEW_RELEASES-PRIMARY":   "Novedades Primaria",
	"NEW_RELEASES-SECONDARY": "Novedades Secundaria",
	"NOVELAS":                "Novelas",
	"CUENTOS":                "Cuentos",
	"EDUCATIONAL_TEXTS":      "Textos educativos",
	"INICIAL":                "Inicial",
	"PRIMARIA":               "Primaria",
	"SECUNDARIA":             "Secundaria",
	"EDUCACION_FISICA":       "Educación Física",
	"LITERATURA":             "Literatura",
	"PSICOLOGIA":             "Psicología",
	"ARTES_PLASTICAS":        "Artes Plásticas",
	"MUSICA":                 "Música",
	"VALORES":                "Valores",
	"OBRA_LITERARIA":         "Obra literaria",
	"TEXTO_EDUCATIVO":        "Texto educativo",
	"ESPANOL":                "Español",
	"INGLES":                 "Inglés",
}

func main() {
	store, err := loadStore()
	if err != nil {
		log.Fatal(err)
	}
	store.BaseURL = envOrDefault("OTERO_BASE_URL", "https://oteroediciones.com")
	addr := envOrDefault("OTERO_ADDR", "127.0.0.1:8080")

	funcs := template.FuncMap{
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict expects key/value pairs")
			}
			result := make(map[string]any, len(values)/2)
			for index := 0; index < len(values); index += 2 {
				key, ok := values[index].(string)
				if !ok {
					return nil, fmt.Errorf("dict key must be a string")
				}
				result[key] = values[index+1]
			}
			return result, nil
		},
		"slice": func(values ...string) []string { return values },
		"label": func(value string) string {
			if translated, ok := translations[value]; ok {
				return translated
			}
			return strings.ReplaceAll(strings.ToLower(value), "_", " ")
		},
		"coverSmall": func(slug string) string {
			return "https://otero-ediciones.s3.amazonaws.com/tapas/small/" + url.PathEscape(slug) + "-tapa.jpg"
		},
		"coverOriginal": func(slug string) string {
			return "https://otero-ediciones.s3.amazonaws.com/tapas/originals/" + url.PathEscape(slug) + "-tapa.jpg"
		},
		"synopsisURL": localSynopsisURL,
		"selected": func(query url.Values, key, option string) bool {
			for _, value := range query[key] {
				for _, item := range strings.Split(value, ",") {
					if normalize(item) == normalize(option) {
						return true
					}
				}
			}
			return false
		},
		"queryString": func(query url.Values, start int) string {
			copy := url.Values{}
			for key, values := range query {
				for _, value := range values {
					copy.Add(key, value)
				}
			}
			copy.Set("startIndex", strconv.Itoa(start))
			return copy.Encode()
		},
	}
	templates := template.Must(template.New("site").Funcs(funcs).ParseFS(siteFiles, "templates/*.html"))

	mux := http.NewServeMux()
	static, err := fs.Sub(siteFiles, "static/assets")
	if err != nil {
		log.Fatal(err)
	}
	assetHandler := http.StripPrefix("/assets/", http.FileServer(http.FS(static)))
	mux.Handle("/assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cacheControl := "public, max-age=604800"
		if strings.HasSuffix(r.URL.Path, ".mp4") || strings.HasSuffix(r.URL.Path, ".jpg") ||
			strings.HasSuffix(r.URL.Path, ".png") || strings.HasSuffix(r.URL.Path, ".svg") ||
			strings.HasSuffix(r.URL.Path, ".woff") || strings.HasSuffix(r.URL.Path, ".woff2") ||
			strings.HasSuffix(r.URL.Path, ".ttf") {
			cacheControl = "public, max-age=2592000"
		}
		if strings.HasSuffix(r.URL.Path, "/app.css") {
			cacheControl = "public, max-age=86400"
		}
		w.Header().Set("Cache-Control", cacheControl)
		assetHandler.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/", store.homeHandler(templates))
	mux.HandleFunc("/catalogo", store.catalogHandler(templates))
	mux.HandleFunc("/catalogo/", store.bookHandler(templates))
	mux.HandleFunc("/historia", store.historyHandler(templates))
	mux.HandleFunc("/sitemap.xml", store.sitemapHandler)
	mux.HandleFunc("/api/home", store.homeJSONHandler)
	mux.HandleFunc("/api/catalogo", store.catalogJSONHandler)
	mux.HandleFunc("/api/catalogo/", store.bookJSONHandler)

	server := &http.Server{
		Addr:              addr,
		Handler:           logging(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Otero Ediciones running at http://%s (%d books)", server.Addr, len(store.Books))
	log.Fatal(server.ListenAndServe())
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func loadStore() (Store, error) {
	booksFile, err := siteFiles.Open("books.tsv")
	if err != nil {
		return Store{}, fmt.Errorf("open books.tsv: %w", err)
	}
	defer booksFile.Close()

	reader := csv.NewReader(booksFile)
	reader.Comma = '\t'
	record, err := reader.Read()
	if err != nil {
		return Store{}, fmt.Errorf("read books.tsv header: %w", err)
	}
	if len(record) < 15 {
		return Store{}, fmt.Errorf("books.tsv header has %d columns, expected 15", len(record))
	}

	var books []Book
	for row := 2; ; row++ {
		record, err = reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Store{}, fmt.Errorf("read books.tsv row %d: %w", row, err)
		}
		if len(record) < 15 {
			return Store{}, fmt.Errorf("books.tsv row %d has %d columns, expected 15", row, len(record))
		}
		books = append(books, Book{
			SimplifiedName: record[0], Titulo: record[1], Autor: record[2],
			NivelEducativo: record[3], Materia: record[4], Tipo: record[5],
			Idioma: record[6], Ilustraciones: record[7], Genero: record[8],
			Paginas: record[9], Tamano: record[10], DepositoLegal: record[11],
			ISBN: record[12], Edad: record[13], FichaDidactica: record[14],
		})
	}

	categoriesFile, err := siteFiles.Open("frontpage_categories.json")
	if err != nil {
		return Store{}, fmt.Errorf("open frontpage_categories.json: %w", err)
	}
	defer categoriesFile.Close()
	var categories []Category
	if err := json.NewDecoder(categoriesFile).Decode(&categories); err != nil {
		return Store{}, fmt.Errorf("decode frontpage_categories.json: %w", err)
	}

	bySlug := make(map[string]Book, len(books))
	for _, book := range books {
		bySlug[book.SimplifiedName] = book
	}
	return Store{Books: books, Categories: categories, BySlug: bySlug}, nil
}

func (s Store) homeRows() []HomeRow {
	rows := make([]HomeRow, 0, len(s.Categories))
	for _, category := range s.Categories {
		row := HomeRow{Titulo: category.Titulo}
		for _, slug := range category.Libros {
			if book, ok := s.BySlug[slug]; ok {
				row.Books = append(row.Books, book)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (s Store) search(query url.Values) CatalogResult {
	start := parseStart(query.Get("startIndex"))
	levels := queryValues(query, "nivel")
	subjects := queryValues(query, "materia")
	types := queryValues(query, "tipo")
	languages := queryValues(query, "idioma")
	term := normalize(query.Get("busqueda"))
	if term == "" {
		term = normalize(query.Get("search"))
	}

	matched := 0
	result := CatalogResult{StartIndex: start, NextStartIndex: start + pageSize}
	for _, book := range s.Books {
		if !matches(book.NivelEducativo, levels) || !matches(book.Materia, subjects) ||
			!matches(book.Tipo, types) || !matches(book.Idioma, languages) {
			continue
		}
		if term != "" && !searchMatches(term, book) {
			continue
		}
		if matched < start {
			matched++
			continue
		}
		if len(result.Books) == pageSize {
			result.HasMore = true
			break
		}
		result.Books = append(result.Books, book)
	}
	return result
}

func searchMatches(term string, book Book) bool {
	haystack := normalize(strings.Join([]string{book.Titulo, book.Autor, book.SimplifiedName}, " "))
	for _, word := range strings.Fields(term) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

func queryValues(query url.Values, key string) []string {
	var values []string
	for _, raw := range query[key] {
		values = append(values, strings.Split(raw, ",")...)
	}
	return values
}

func matches(value string, options []string) bool {
	if len(options) == 0 {
		return true
	}
	value = normalize(value)
	for _, option := range options {
		if value == normalize(option) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"_", " ", "-", " ", ".", " ",
	).Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func parseStart(raw string) int {
	start, err := strconv.Atoi(raw)
	if err != nil || start < 0 {
		return 0
	}
	return start
}

func (s Store) homeHandler(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		render(w, t, "home", Page{
			Title:       "Otero Ediciones | Libros educativos y literatura infantil",
			Description: "Editorial boliviana especializada en libros educativos y literatura infantil.",
			Path:        r.URL.Path, HomeRows: s.homeRows(), Query: r.URL.Query(),
		})
	}
}

func (s Store) catalogHandler(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalogo" {
			http.NotFound(w, r)
			return
		}
		page := Page{
			Title:       "Catálogo de libros | Otero Ediciones",
			Description: "Explora el catálogo de libros educativos y cuentos infantiles de Otero Ediciones.",
			Path:        r.URL.Path, Query: r.URL.Query(), Catalog: s.search(r.URL.Query()),
		}
		if r.Header.Get("HX-Request") == "true" {
			render(w, t, "catalog-fragment", page)
			return
		}
		render(w, t, "catalog", page)
	}
}

func (s Store) bookHandler(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/catalogo/")
		if strings.HasSuffix(slug, "/sinopsis") {
			s.synopsisHandler(w, r, strings.TrimSuffix(slug, "/sinopsis"))
			return
		}
		book, ok := s.BySlug[slug]
		if !ok || strings.Contains(slug, "/") || slug == "" {
			http.NotFound(w, r)
			return
		}
		synopsis := ""
		if s.synopsisExists(slug) {
			synopsis = localSynopsisURL(slug)
		}
		render(w, t, "book", Page{
			Title: book.Titulo + " | Otero Ediciones", Description: book.Titulo,
			Path: r.URL.Path, Book: &book, SynopsisURL: synopsis, Query: r.URL.Query(),
		})
	}
}

func localSynopsisURL(slug string) string {
	return "/catalogo/" + url.PathEscape(slug) + "/sinopsis"
}

func synopsisFilePath(slug string) string {
	return "content/synopsis/" + slug + ".txt"
}

func (s Store) synopsisExists(slug string) bool {
	file, err := siteFiles.Open(synopsisFilePath(slug))
	if err != nil {
		return false
	}
	file.Close()
	return true
}

func (s Store) synopsisHandler(w http.ResponseWriter, r *http.Request, slug string) {
	if _, ok := s.BySlug[slug]; !ok || !s.synopsisExists(slug) {
		http.NotFound(w, r)
		return
	}
	file, err := siteFiles.Open(synopsisFilePath(slug))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("serve synopsis %s: %v", slug, err)
	}
}

func (s Store) historyHandler(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/historia" {
			http.NotFound(w, r)
			return
		}
		render(w, t, "history", Page{
			Title: "Nosotros | Otero Ediciones", Description: "Conoce la historia y propuesta de Otero Ediciones.",
			Path: r.URL.Path, Query: r.URL.Query(),
		})
	}
}

func render(w http.ResponseWriter, t *template.Template, name string, page Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, page); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

func (s Store) homeJSONHandler(w http.ResponseWriter, r *http.Request) {
	rows := make([]map[string]any, 0, len(s.Categories))
	for _, row := range s.homeRows() {
		books := make([]map[string]string, 0, len(row.Books))
		for _, book := range row.Books {
			books = append(books, map[string]string{
				"titulo": book.Titulo, "simplified_name": book.SimplifiedName,
				"tapa_small": "https://otero-ediciones.s3.amazonaws.com/tapas/small/" + url.PathEscape(book.SimplifiedName) + "-tapa.jpg",
			})
		}
		rows = append(rows, map[string]any{"titulo": row.Titulo, "book_responses": books})
	}
	writeJSON(w, rows)
}

func (s Store) catalogJSONHandler(w http.ResponseWriter, r *http.Request) {
	result := s.search(r.URL.Query())
	responses := make([]map[string]string, 0, len(result.Books))
	for _, book := range result.Books {
		responses = append(responses, map[string]string{
			"titulo": book.Titulo, "simplified_name": book.SimplifiedName,
			"tapa_small": "https://otero-ediciones.s3.amazonaws.com/tapas/small/" + url.PathEscape(book.SimplifiedName) + "-tapa.jpg",
		})
	}
	writeJSON(w, responses)
}

func (s Store) bookJSONHandler(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/catalogo/")
	book, ok := s.BySlug[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	response := map[string]string{
		"titulo": book.Titulo, "tapa_original_url": "https://otero-ediciones.s3.amazonaws.com/tapas/originals/" + url.PathEscape(slug) + "-tapa.jpg",
		"autor": book.Autor, "ilustraciones": book.Ilustraciones, "materia": book.Materia,
		"nivel_educativo": book.NivelEducativo, "genero": book.Genero, "guia_didactica": book.FichaDidactica,
		"tamano": book.Tamano, "paginas": book.Paginas, "isbn": book.ISBN, "deposito_legal": book.DepositoLegal,
		"tipo": book.Tipo, "idioma": book.Idioma,
	}
	if s.synopsisExists(slug) {
		response["descripcion"] = localSynopsisURL(slug)
	}
	writeJSON(w, response)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
}

func (s Store) sitemapHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	baseURL := strings.TrimRight(s.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://oteroediciones.com"
	}
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, path := range []string{"/", "/catalogo", "/historia"} {
		fmt.Fprintf(w, "<url><loc>%s%s</loc></url>", baseURL, path)
	}
	for _, book := range s.Books {
		fmt.Fprintf(w, "<url><loc>%s/catalogo/%s</loc></url>", baseURL, url.PathEscape(book.SimplifiedName))
	}
	fmt.Fprint(w, `</urlset>`)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.RequestURI(), time.Since(started).Round(time.Millisecond))
	})
}
