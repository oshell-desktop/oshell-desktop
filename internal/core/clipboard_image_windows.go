package core

// Windows uses the data directory's inherited ACL rather than Unix mode bits.
func validateImageCacheDirectory(string) error { return nil }
