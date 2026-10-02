# Otero Ediciones — unified app

This is the first unified version of the Otero Ediciones site. It replaces the separate Angular frontend and Go JSON server with one server-rendered Go application:

- Go templates render `/`, `/catalogo`, `/catalogo/:slug`, and `/historia`.
- HTMX updates catalog results and load-more pages without a client-side router.
- Alpine.js manages small UI interactions such as collapsible filters.
- Tailwind CSS provides the responsive layout and visual styling.
- The TSV catalog, homepage categories, public assets, and local synopsis text are embedded into the binary.
- The existing JSON routes remain available under `/api` while the migration settles.

## Run locally

From this directory:

```sh
./run.sh
```

This rebuilds the production Tailwind stylesheet and starts the Go app. Then open <http://localhost:8080>.

To build the production CSS and a self-contained binary:

```sh
tailwindcss -i styles/input.css -o static/assets/app.css --minify
go build -o otero-ediciones .
./otero-ediciones
```

Run the tests with:

```sh
go test ./...
```

## Search behavior

Search is handled in the shared Go data layer. It is case-insensitive, ignores Spanish accents, treats spaces/underscores/hyphens consistently, and searches titles, authors, and slugs. This avoids the previous mismatch where Angular normalized a query differently from the backend.

## Content and media strategy

Book synopsis text is stored under `content/synopsis` and embedded into the Go binary. Detail pages load it through a same-origin endpoint, avoiding S3 CORS and permission problems. The current snapshot contains 68 non-empty synopsis files and is only about 62 KB. Books without a local synopsis do not render a broken synopsis section.

Book covers remain on S3 and are lazy-loaded by the browser. Covers are media assets, not application data; keeping them behind S3/CDN avoids making the Go binary and every deployment unnecessarily large. A local image mirror would make sense only if the S3 bucket is being retired, needs stronger availability guarantees, or the site must run fully offline.

## Styling note

Tailwind is compiled locally with the Tailwind CLI v4. The source is `styles/input.css`, which scans the Go templates, and the generated `static/app.css` is embedded and served by the Go application. There is no Tailwind CDN runtime dependency in production.

The original `frontend` and `backend` directories are intentionally left unchanged; this directory is the migration target.
