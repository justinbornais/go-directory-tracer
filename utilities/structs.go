package utilities

type Folder struct {
	Name     string
	Size     string
	Modified string
}

type File struct {
	Name      string
	Extension string
	Size      string
	Modified  string
}

type SearchEntry struct {
	Name string
	Type string // "f" or "d"
	Path string // relative path from root using forward slashes; empty = root
}
