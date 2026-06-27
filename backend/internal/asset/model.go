package asset

type Asset struct {
	ID             uint64
	GroupID        uint64
	Category       string
	Title          string
	OriginalName   string
	StoragePath    string
	MimeType       string
	FileSize       uint64
	ChecksumSHA256 string
	Visibility     string
}

type StoredObject struct {
	StoragePath    string
	FileSize       uint64
	ChecksumSHA256 string
}

type ResolvedObject struct {
	AbsolutePath string
	OriginalName string
}

type DownloadFile struct {
	AbsolutePath string
	OriginalName string
	MimeType     string
}
