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

The server accepts these environment variables:

- `OTERO_ADDR` — bind address; defaults to `127.0.0.1:8080`.
- `OTERO_BASE_URL` — canonical public URL used by the sitemap; defaults to `https://oteroediciones.com`.

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

Tailwind is compiled locally with the Tailwind CLI v4. The source is `styles/input.css`, which scans the Go templates, and the generated `static/assets/app.css` is embedded and served by the Go application. Montserrat is served locally as WOFF2 from `static/assets/fonts`, so the page does not depend on Google Fonts. There is no Tailwind CDN runtime dependency in production.

## VPS deployment

Example systemd and Caddy configurations are in `deploy/`. The intended shared-VPS setup is to run this app on `127.0.0.1:8091` and let the existing reverse proxy terminate HTTPS for `oteroediciones.com`. The `/healthz` endpoint can be used by the service monitor or reverse proxy.

To build and deploy an update from this directory:

```sh
./update.sh
```

The script runs tests, rebuilds Tailwind, cross-compiles the Linux binary, uploads it over SSH, atomically installs it, restarts the systemd service, and checks `/healthz`. Override the defaults when needed with `OTERO_DEPLOY_TARGET`, `OTERO_REMOTE_DIR`, or `OTERO_SERVICE`.

## License

This unified application is licensed under the GNU General Public License, version 2.0. See [`LICENSE`](LICENSE).

The included Otero Ediciones branding, catalog content, synopsis text, and external book-cover media may have separate rights and should not be assumed to be covered by the software license unless explicitly authorized.

The original `frontend` and `backend` directories are intentionally left unchanged; this directory is the migration target.
