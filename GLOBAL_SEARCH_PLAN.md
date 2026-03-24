# Global File Search — Implementation Plan

## Overview

Currently, each generated `index.html` contains a local `const d = [...]` array and uses Fuse.js to search only within that single directory. This plan extends the tracer to support **cross-directory search**: a user can search by name and find files/folders living anywhere in the tree, with a direct link to each result.

---

## How the Current Architecture Works (Relevant Parts)

| Layer | What it does |
|---|---|
| `IndexFolder` (Go) | Recursively visits every directory, emits one `index.html` per folder |
| `const d = [...]` | Inline JSON array baked into each `index.html` at build time; contains only that folder's immediate children |
| Fuse.js (CDN) | Client-side fuzzy search over `d`; scope is strictly local |
| `data.json` (optional) | Same per-directory data written to a separate JSON file with `--json` |

---

## What Global Search Requires

### 1. A flat, root-level search index

During the recursive traversal, every file and folder entry (with its full relative path from the root) must be aggregated into a single structure. The minimal schema per entry:

```json
{ "n": "filename.pdf", "t": "f", "p": "docs/legal" }
```

- `n` — name (existing field)
- `t` — type: `"f"` file or `"d"` folder (existing field)
- `p` — **new**: path relative to root, e.g. `"music/2024/rock"`. An empty string means the root.

This index gets either:
- **Embedded inline** inside `search.html` as `const sd = [...]` (zero extra HTTP requests, fully self-contained — recommended), or
- Written to a standalone `search-index.json` at the root and fetched by `search.html` via `fetch()` (one extra HTTP request but lighter HTML).

The inline approach is consistent with how every `index.html` already works, keeps everything static, and avoids any CORS/fetch complications on `file://` URLs.

### 2. A dedicated `search.html` at the root

A new static page that:

- Loads Fuse.js from the same CDN already used.
- Contains the aggregated `const sd = [...]` inline.
- Renders results with a clickable link resolved as `p/n` (e.g. `music/2024/rock/song.mp3`).
- Shows the path visually so users know where each result lives.
- Reuses the same CSS already embedded in every `index.html`.

### 3. A link to `search.html` in every `index.html`

Each per-directory `index.html` gets a "Global Search" anchor in its `#top` header. The href must account for depth:

- Root `index.html` → `href="search.html"`
- One level deep → `href="../search.html"`
- Two levels deep → `href="../../search.html"`

This is already a solved pattern in the codebase — `recurse.go` already computes `base := strings.Repeat("../", depth)` which can be reused directly as `base + "search.html"`.

### 4. New CLI flag

```sh
--global-search   Generate a root-level search.html and embedded search index
```

This keeps the feature opt-in and doesn't affect existing output for users who don't pass the flag.

---

## Go Changes Required

### `utilities/structs.go`
Add a new entry type for the global index:

```go
type SearchEntry struct {
    Name string
    Type string // "f" or "d"
    Path string // relative path from root, empty = root
}
```

### `utilities/recurse.go` — `IndexFolder`

Two options:

**Option A — Pass an accumulator slice (pointer)**  
Add a `*[]SearchEntry` parameter. Each call appends its filtered entries before recursing. At the top-level call (depth 0), write the final result.

**Option B — Two-pass: separate `BuildSearchIndex` function**  
After `IndexFolder` finishes, call a second function that walks the tree again and collects entries. Simpler to keep `IndexFolder`'s signature stable.

Option A is more efficient (single traversal) and preferred.

### `utilities/html.go`

1. Add `GenerateSearchHTML(title, css, js, indexData string) string` — builds the `search.html` boilerplate with `const sd = [indexData]` embedded.
2. Add an optional `[global-search]` placeholder to the existing boilerplate, which is replaced with either a link or an empty string depending on the flag.

### `utilities/jsondata.go`

Add `WriteSearchEntryJSON(entries []SearchEntry) string` — mirrors the existing `WriteFolderJSON` / `WriteFileJSON` pattern.

### `main.go`

```go
globalSearch := flag.Bool("global-search", false, "Generate a root-level search.html with a cross-directory search index")
```

Pass the accumulator into `IndexFolder`. After it returns, call the new function to write `search.html`.

### `static/` — new `search.js`

A small JS file (embedded alongside `default.js`) that:

```js
const fuse = new Fuse(sd, { keys: ['n'], /* same options */ });

function buildResults(q) { /* render results with path shown */ }
document.getElementById("q").addEventListener("input", debounce(...));
```

Because it's embedded via `embed.FS` and inlined into `search.html` at build time, there is no additional static file dependency at runtime.

---

## Resulting File Output

```
(root)/
├── index.html          ← unchanged structure, + "Global Search" link
├── search.html         ← NEW: global search page, fully self-contained
├── subdir/
│   ├── index.html      ← unchanged structure, + "Global Search" link
│   └── ...
└── ...
```

No `search-index.json` is written separately; the data lives inside `search.html`. This matches the existing philosophy perfectly.

---

## Difficulty Assessment

| Task | Effort | Notes |
|---|---|---|
| Add `SearchEntry` struct + JSON serializer | Very low | Mirrors existing `Folder`/`File` pattern |
| Thread accumulator through `IndexFolder` | Low | One extra parameter, same recursion pattern |
| `GenerateSearchHTML` in `html.go` | Low | Copy of existing boilerplate function |
| `search.js` static file | Low–Medium | New file; needs path-aware link building |
| Link injection in each `index.html` | Low | `[base]` already computed per depth |
| CLI flag + wiring in `main.go` | Very low | Matches existing flag pattern |
| **Total** | **Low–Medium** | Estimate: a few hours of focused work |

The hardest part is **correctness of relative paths** in search results, since `search.html` lives at the root and result links must be `rootRelativePath/filename`. This is straightforward but requires careful string handling. The second consideration is **index size**: for very large trees (tens of thousands of files), the inline JSON in `search.html` could become large. This is a known trade-off of the static approach and is acceptable for the typical use-case of this tool.

---

## Preserving the "Minimized Static HTML Dependencies" Principle

| Principle | How it is maintained |
|---|---|
| No new runtime dependencies | Fuse.js CDN is already used; `search.js` is embedded inline |
| No server required | `search.html` is a self-contained static file |
| No extra HTTP requests | Search index is inlined as `const sd = [...]`, not fetched |
| Opt-in only | `--global-search` flag; existing output unchanged without it |
| Works on GitHub Pages | Static files only; no API calls or server-side logic |
| Works on `file://` | No `fetch()` calls; all data is inline |

---

## Deployment Steps

### Local Binary

```sh
# Build (existing commands, unchanged)
$Env:GOOS = "linux"
go build -ldflags='-s -w' -trimpath -o tracer

$Env:GOOS = "windows"
go build -ldflags='-s -w' -trimpath -o tracer.exe

# Run with global search enabled
./tracer --title "My Site" --global-search
```

This produces `search.html` at the root alongside all the `index.html` files.

### GitHub Pages (`.github/workflows/ghpages.yml`)

Add `--global-search` to the tracer invocation step in the workflow. No other workflow changes are needed — `search.html` is just another generated static file that gets uploaded to Pages along with all the `index.html` files.

```yaml
- name: Run tracer
  run: ./tracer --title "${{ env.SITE_TITLE }}" --global-search
```

---

## Optional Enhancements (Out of Scope for v1)

- **Scope toggle**: A checkbox on `search.html` to limit results to a specific subtree (e.g. "search only under `music/`").
- **Result ranking by recency**: Include the `m` (modified) field in `SearchEntry` when `--details` is also passed, and sort/display it in results.
- **Link from `search.html` back to the current directory**: Not meaningful since search is always run from the root page.
- **Lazy-loaded index**: Write `search-index.json` separately and `fetch()` it in `search.html` as an alternative for very large trees where inlining is impractical. Could be an additional flag like `--global-search-external`.
