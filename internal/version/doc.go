// Package version reports Foundry identity from Go build settings (REQ-162,
// Section 41): semantic version, VCS commit, Go toolchain string, and the
// embedded catalog digest.
//
// Release binaries embed metadata via build settings (module version / VCS
// stamping), not linker-flag string injection. Development builds fall back
// to DefaultVersion when Main.Version is empty or "(devel)".
package version
