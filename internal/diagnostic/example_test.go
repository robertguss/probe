package diagnostic_test

import (
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// ExampleNew shows constructing a FoundryError with a stable identifier.
func ExampleNew() {
	err := diagnostic.New(diagnostic.IDSpecInvalidField, "name must be kebab-case", diagnostic.SpecLocation("foundry.toml", 3, 1))
	fmt.Println(err.ID())
	fmt.Println(err.ExitCode())
	// Output:
	// spec.invalid_field
	// 2
}
