package asset

// Windows local development flushes file contents; opening a directory with
// os.Open does not provide a handle on which FlushFileBuffers is supported.
func syncResourceDirectories(_, _ string) error { return nil }
