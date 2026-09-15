// ABOUTME: Detects Linux security metadata that cannot be copied as ordinary attributes.
// ABOUTME: Retains ACL refusal while allowing user metadata to survive atomic replacement.
package annotation

import "os"

func replacementMetadata(f *os.File) error {
	attrs, err := readFileAttributes(f)
	if err != nil {
		return err
	}
	for name := range attrs {
		if name == "system.posix_acl_access" || name == "system.posix_acl_default" {
			return fail("storage_metadata_unsupported")
		}
	}
	return nil
}
